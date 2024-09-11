package modules

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	cgutilcert "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"

	"github.com/kubesphere/kubekey/v4/pkg/variable"
)

const (
	// DefaultSignCertAfter defines the default timeout for sign certificates.
	defaultSignCertAfter = time.Hour * 24 * 365 * 10
	// CertificateBlockType is a possible value for pem.Block.Type.
	certificateBlockType = "CERTIFICATE"
	rsaKeySize           = 2048

	// policy to generate file
	// policyAlways always generate new cert to override exist cert
	policyAlways = "Always"
	// policyIfNotPresent if cert is exist, check it.if not generate new cert.
	policyIfNotPresent = "IfNotPresent"
)

// ModuleGenCert generate cert file.
// if root_key and root_cert is empty, generate Self-signed certificate.
func ModuleGenCert(ctx context.Context, options ExecOptions) (stdout string, stderr string) {
	// get host variable
	ha, err := options.Variable.Get(variable.GetAllVariable(options.Host))
	if err != nil {
		return "", fmt.Sprintf("failed to get host variable: %v", err)
	}

	// args
	args := variable.Extension2Variables(options.Args)
	rootKeyParam, _ := variable.StringVar(ha.(map[string]any), args, "root_key")
	rootCertParam, _ := variable.StringVar(ha.(map[string]any), args, "root_cert")
	dateParam, _ := variable.StringVar(ha.(map[string]any), args, "date")
	policyParam, _ := variable.StringVar(ha.(map[string]any), args, "policy")
	sansParam, _ := variable.StringSliceVar(ha.(map[string]any), args, "sans")
	cnParam, _ := variable.StringVar(ha.(map[string]any), args, "cn")
	outKeyParam, _ := variable.StringVar(ha.(map[string]any), args, "out_key")
	outCertParam, _ := variable.StringVar(ha.(map[string]any), args, "out_cert")
	// check args
	if policyParam != policyAlways && policyParam != policyIfNotPresent {
		return "", "\"policy\" should be one of [Always, IfNotPresent]"
	}
	if outKeyParam == "" || outCertParam == "" {
		return "", "\"out_key\" or \"out_cert\" in args should be string"
	}
	if cnParam == "" {
		return "", "\"cn\" in args should be string"
	}

	altName := &cgutilcert.AltNames{
		DNSNames: []string{"localhost"},
		IPs:      []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	appendSANsToAltNames(altName, sansParam, outCertParam)
	cfg := &cgutilcert.Config{
		CommonName:   cnParam,
		Organization: []string{"kubekey"},
		AltNames:     *altName,
	}

	var newKey *rsa.PrivateKey
	var newCert *x509.Certificate
	newKey, err = rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
	if err != nil {
		return "", fmt.Sprintf("generate rsa key error: %v", err)
	}

	var after time.Duration
	// change expiration date
	if dateParam != "" {
		dur, err := time.ParseDuration(dateParam)
		if err != nil {
			return "", fmt.Sprintf("failed to parse duration: %v", err)
		}
		after = dur
	}

	switch {
	case rootKeyParam == "" || rootCertParam == "": // generate Self-signed certificate
		newCert, err = NewSelfSignedCACert(*cfg, after, newKey)
		if err != nil {
			return "", fmt.Sprintf("failed to generate Self-signed certificate: %v", err)
		}
	default: // generate certificate signed by root certificate
		parentKey, err := TryLoadKeyFromDisk(rootKeyParam)
		if err != nil {
			return "", fmt.Sprintf("failed to load root key: %v", err)
		}
		parentCert, _, err := TryLoadCertChainFromDisk(rootCertParam)
		if err != nil {
			return "", fmt.Sprintf("failed to load root certificate: %v", err)
		}
		if policyParam == policyIfNotPresent {
			if _, err := TryLoadKeyFromDisk(outKeyParam); err != nil {
				klog.V(4).InfoS("Failed to load out key, new it")
				goto NEW
			}
			existCert, intermediates, err := TryLoadCertChainFromDisk(outCertParam)
			if err != nil {
				klog.V(4).InfoS("Failed to load out cert, new it")
				goto NEW
			}
			// check if the existing key and cert match the root key and cert
			if err := ValidateCertPeriod(existCert, 0); err != nil {
				return "", fmt.Sprintf("failed to ValidateCertPeriod: %v", err)
			}
			if err := VerifyCertChain(existCert, intermediates, parentCert); err != nil {
				return "", fmt.Sprintf("failed to VerifyCertChain: %v", err)
			}
			if err := validateCertificateWithConfig(existCert, outCertParam, cfg); err != nil {
				return "", fmt.Sprintf("failed to validateCertificateWithConfig: %v", err)
			}
			return stdoutSkip, ""
		}
	NEW:
		newCert, err = NewSignedCert(*cfg, after, newKey, parentCert, parentKey, true)
		if err != nil {
			return "", fmt.Sprintf("failed to generate certificate: %v", err)
		}
	}

	// write key and cert to file
	if err := WriteKey(outKeyParam, newKey, policyParam); err != nil {
		return "", fmt.Sprintf("failed to write key: %v", err)
	}
	if err := WriteCert(outCertParam, newCert, policyParam); err != nil {
		return "", fmt.Sprintf("failed to write certificate: %v", err)
	}
	return stdoutSuccess, ""
}

// WriteKey stores the given key at the given location
func WriteKey(outKey string, key crypto.Signer, policy string) error {
	if _, err := os.Stat(outKey); err == nil && policy == policyIfNotPresent {
		// skip
		return nil
	}
	if key == nil {
		return errors.New("private key cannot be nil when writing to file")
	}

	encoded, err := keyutil.MarshalPrivateKeyToPEM(key)
	if err != nil {
		return fmt.Errorf("unable to marshal private key to PEM, error: %w", err)
	}
	if err := keyutil.WriteKey(outKey, encoded); err != nil {
		return fmt.Errorf("unable to write private key to file %s, error: %w", outKey, err)
	}

	return nil
}

// WriteCert stores the given certificate at the given location
func WriteCert(outCert string, cert *x509.Certificate, policy string) error {
	if _, err := os.Stat(outCert); err == nil && policy == policyIfNotPresent {
		// skip
		return nil
	}
	if cert == nil {
		return errors.New("certificate cannot be nil when writing to file")
	}

	if err := cgutilcert.WriteCert(outCert, EncodeCertPEM(cert)); err != nil {
		return fmt.Errorf("unable to write certificate to file %s, error: %w", outCert, err)
	}

	return nil
}

// EncodeCertPEM returns PEM-endcoded certificate data
func EncodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  certificateBlockType,
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// TryLoadKeyFromDisk tries to load the key from the disk and validates that it is valid
func TryLoadKeyFromDisk(rootKey string) (crypto.Signer, error) {
	// Parse the private key from a file
	privKey, err := keyutil.PrivateKeyFromFile(rootKey)
	if err != nil {
		return nil, fmt.Errorf("couldn't load the private key file %s, error: %w", rootKey, err)
	}

	// Allow RSA and ECDSA formats only
	var key crypto.Signer
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		key = k
	case *ecdsa.PrivateKey:
		key = k
	default:
		return nil, fmt.Errorf("the private key file %s is neither in RSA nor ECDSA format", rootKey)
	}

	return key, nil
}

// TryLoadCertChainFromDisk tries to load the cert chain from the disk
func TryLoadCertChainFromDisk(rootCert string) (*x509.Certificate, []*x509.Certificate, error) {
	certs, err := cgutilcert.CertsFromFile(rootCert)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't load the certificate file %s, error: %w", rootCert, err)
	}

	cert := certs[0]
	intermediates := certs[1:]

	return cert, intermediates, nil
}

// appendSANsToAltNames parses SANs from as list of strings and adds them to altNames for use on a specific cert
// altNames is passed in with a pointer, and the struct is modified
// valid IP address strings are parsed and added to altNames.IPs as net.IP's
// RFC-1123 compliant DNS strings are added to altNames.DNSNames as strings
// RFC-1123 compliant wildcard DNS strings are added to altNames.DNSNames as strings
// certNames is used to print user facing warnings and should be the name of the cert the altNames will be used for
func appendSANsToAltNames(altNames *cgutilcert.AltNames, sans []string, certName string) {
	for _, altname := range sans {
		if ip := netutils.ParseIPSloppy(altname); ip != nil {
			altNames.IPs = append(altNames.IPs, ip)
		} else if len(validation.IsDNS1123Subdomain(altname)) == 0 {
			altNames.DNSNames = append(altNames.DNSNames, altname)
		} else if len(validation.IsWildcardDNS1123Subdomain(altname)) == 0 {
			altNames.DNSNames = append(altNames.DNSNames, altname)
		} else {
			klog.V(4).Infof(
				"[certificates] WARNING: '%s' was not added to the '%s' SAN, because it is not a valid IP or RFC-1123 compliant DNS entry\n",
				altname,
				certName,
			)
		}
	}
}

// NewSelfSignedCACert creates a CA certificate
func NewSelfSignedCACert(cfg cgutilcert.Config, after time.Duration, key crypto.Signer) (*x509.Certificate, error) {
	now := time.Now()
	// returns a uniform random value in [0, max-1), then add 1 to serial to make it a uniform random value in [1, max).
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	if err != nil {
		return nil, err
	}
	serial = new(big.Int).Add(serial, big.NewInt(1))
	notBefore := now.UTC()
	if !cfg.NotBefore.IsZero() {
		notBefore = cfg.NotBefore.UTC()
	}
	if after == 0 { // default 10 year
		after = defaultSignCertAfter
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:              []string{cfg.CommonName},
		NotBefore:             notBefore,
		NotAfter:              now.Add(after).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func NewSignedCert(cfg cgutilcert.Config, after time.Duration, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer, isCA bool) (*x509.Certificate, error) {
	// returns a uniform random value in [0, max-1), then add 1 to serial to make it a uniform random value in [1, max).
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	if err != nil {
		return nil, err
	}
	serial = new(big.Int).Add(serial, big.NewInt(1))
	if cfg.CommonName == "" {
		return nil, fmt.Errorf("must specify a CommonName")
	}

	keyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	if isCA {
		keyUsage |= x509.KeyUsageCertSign
	}

	RemoveDuplicateAltNames(&cfg.AltNames)

	if after == 0 {
		after = defaultSignCertAfter
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:              cfg.AltNames.DNSNames,
		IPAddresses:           cfg.AltNames.IPs,
		SerialNumber:          serial,
		NotBefore:             caCert.NotBefore,
		NotAfter:              time.Now().Add(after).UTC(),
		KeyUsage:              keyUsage,
		ExtKeyUsage:           cfg.Usages,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// RemoveDuplicateAltNames removes duplicate items in altNames.
func RemoveDuplicateAltNames(altNames *cgutilcert.AltNames) {
	if altNames == nil {
		return
	}

	if altNames.DNSNames != nil {
		altNames.DNSNames = sets.List(sets.New(altNames.DNSNames...))
	}

	ipsKeys := make(map[string]struct{})
	var ips []net.IP
	for _, one := range altNames.IPs {
		if _, ok := ipsKeys[one.String()]; !ok {
			ipsKeys[one.String()] = struct{}{}
			ips = append(ips, one)
		}
	}
	altNames.IPs = ips
}

// ValidateCertPeriod checks if the certificate is valid relative to the current time
// (+/- offset)
func ValidateCertPeriod(cert *x509.Certificate, offset time.Duration) error {
	period := fmt.Sprintf("NotBefore: %v, NotAfter: %v", cert.NotBefore, cert.NotAfter)
	now := time.Now().Add(offset)
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("the certificate is not valid yet: %s", period)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("the certificate has expired: %s", period)
	}
	return nil
}

// VerifyCertChain verifies that a certificate has a valid chain of
// intermediate CAs back to the root CA
func VerifyCertChain(cert *x509.Certificate, intermediates []*x509.Certificate, root *x509.Certificate) error {
	rootPool := x509.NewCertPool()
	rootPool.AddCert(root)

	intermediatePool := x509.NewCertPool()
	for _, c := range intermediates {
		intermediatePool.AddCert(c)
	}

	verifyOptions := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: intermediatePool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	if _, err := cert.Verify(verifyOptions); err != nil {
		return err
	}

	return nil
}

// validateCertificateWithConfig makes sure that a given certificate is valid at
// least for the SANs defined in the configuration.
func validateCertificateWithConfig(cert *x509.Certificate, baseName string, cfg *cgutilcert.Config) error {
	for _, dnsName := range cfg.AltNames.DNSNames {
		if err := cert.VerifyHostname(dnsName); err != nil {
			return fmt.Errorf("certificate %s is invalid, error: %w", baseName, err)
		}
	}
	for _, ipAddress := range cfg.AltNames.IPs {
		if err := cert.VerifyHostname(ipAddress.String()); err != nil {
			return fmt.Errorf("certificate %s is invalid, error: %w", baseName, err)
		}
	}
	return nil
}
