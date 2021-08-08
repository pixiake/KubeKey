package certs

import (
	"encoding/base64"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/kubesphere/kubekey/pkg/kubernetes"
	"github.com/kubesphere/kubekey/pkg/util/executor"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	certutil "k8s.io/client-go/util/cert"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type Certificate struct {
	Name          string
	Expires       string
	Residual      string
	AuthorityName string
	NodeName      string
}

type CaCertificate struct {
	AuthorityName string
	Expires       string
	Residual      string
	NodeName      string
}

const (
	kubernetesDir = "/etc/kubernetes/"
	certDir       = kubernetesDir + "pki/"
)

var (
	certificateList = []string{
		"apiserver.crt",
		"apiserver-kubelet-client.crt",
		"front-proxy-client.crt",
	}
	caCertificateList = []string{
		"ca.crt",
		"front-proxy-ca.crt",
	}
	kubeConfigList = []string{
		"admin.conf",
		"controller-manager.conf",
		"scheduler.conf",
	}
	certificates    []*Certificate
	caCertificates  []*CaCertificate
	kubeConfigValue = map[string]string{}
)

var kubeadmList = []string{
	"cd /etc/kubernetes",
	"/usr/local/bin/kubeadm alpha certs renew apiserver",
	"/usr/local/bin/kubeadm alpha certs renew apiserver-kubelet-client",
	"/usr/local/bin/kubeadm alpha certs renew front-proxy-client",
	"/usr/local/bin/kubeadm alpha certs renew admin.conf",
	"/usr/local/bin/kubeadm alpha certs renew controller-manager.conf",
	"/usr/local/bin/kubeadm alpha certs renew scheduler.conf",
}

var restartList = []string{
	"docker ps -af name=k8s_kube-apiserver* -q | xargs --no-run-if-empty docker rm -f",
	"docker ps -af name=k8s_kube-scheduler* -q | xargs --no-run-if-empty docker rm -f",
	"docker ps -af name=k8s_kube-controller-manager* -q | xargs --no-run-if-empty docker rm -f",
	"systemctl restart kubelet",
}

func ListCluster(clusterCfgFile string, logger *log.Logger, verbose bool) error {
	cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to download cluster config")
	}
	return Execute(executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, false, true, false, false, nil))

}
func RenewClusterCerts(clusterCfgFile string, logger *log.Logger, verbose bool) error {
	cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to download cluster config")
	}
	return ExecuteRenew(executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, false, true, false, false, nil))

}

func Execute(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}
	return ExecTasks(mgr)
}
func ExecuteRenew(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}
	return ExecRenewTasks(mgr)
}

func ExecTasks(mgr *manager.Manager) error {
	listTasks := []manager.Task{
		{Task: ListClusterCerts, ErrMsg: "Failed to list cluster certs."},
	}
	for _, step := range listTasks {
		if err := step.Run(mgr); err != nil {
			_ = errors.Wrap(err, step.ErrMsg)
		}
	}
	mgr.Logger.Infoln("Successful.")
	return nil
}
func ExecRenewTasks(mgr *manager.Manager) error {
	renewTasks := []manager.Task{
		{Task: RenewClusterCert, ErrMsg: "Failed to renew cluster certs."},
		{Task: SyncKubeConfig, ErrMsg: "Failed to sync kubeConfig"},
		{Task: ListClusterCerts, ErrMsg: "Failed to list cluster certs."},
	}
	for _, step := range renewTasks {
		if err := step.Run(mgr); err != nil {
			_ = errors.Wrap(err, step.ErrMsg)
		}
	}
	mgr.Logger.Infoln("Successful.")
	return nil
}

func ListClusterCerts(m *manager.Manager) error {
	m.Logger.Infoln("Listing cluster certs ...")
	if err := m.RunTaskOnMasterNodes(listClusterCerts, true); err != nil {
		return err
	}
	printResult(certificates, caCertificates)
	return nil
}

func listClusterCerts(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	for _, certFileName := range certificateList {
		certPath := fmt.Sprintf("%s%s", certDir, certFileName)
		certContext, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"cat %s\"", certPath), 1, false)
		if err != nil {
			return errors.Wrap(err, "Failed to get cluster certs")
		}
		if cert, err := getCertInfo(certContext, certFileName, node.Name); err != nil {
			return err
		} else {
			certificates = append(certificates, cert)
		}
	}

	for _, kubeConfigFileName := range kubeConfigList {
		kubeConfigPath := fmt.Sprintf("%s%s", kubernetesDir, kubeConfigFileName)
		newConfig := clientcmdapi.NewConfig()
		kubeconfigBytes, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"cat %s\"", kubeConfigPath), 1, false)
		decoded, _, err := clientcmdlatest.Codec.Decode([]byte(kubeconfigBytes), &schema.GroupVersionKind{Version: clientcmdlatest.Version, Kind: "Config"}, newConfig)
		if err != nil {
			return err
		}
		newConfig = decoded.(*clientcmdapi.Config)
		for _, a := range newConfig.AuthInfos {
			certContextBase64 := a.ClientCertificateData
			tmp := base64.StdEncoding.EncodeToString(certContextBase64)
			certContext, err := base64.StdEncoding.DecodeString(tmp)
			if err != nil {
				return err
			}
			if cert, err := getCertInfo(string(certContext), kubeConfigFileName, node.Name); err != nil {
				return err
			} else {
				certificates = append(certificates, cert)
			}
		}
	}

	for _, caCertFileName := range caCertificateList {
		certPath := fmt.Sprintf("%s%s", certDir, caCertFileName)
		caCertContext, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"cat %s\"", certPath), 1, false)
		if err != nil {
			return errors.Wrap(err, "Failed to get cluster certs")
		}
		if cert, err := getCacertInfo(caCertContext, caCertFileName, node.Name); err != nil {
			return err
		} else {
			caCertificates = append(caCertificates, cert)
		}
	}

	return nil
}

func printResult(certificates []*Certificate, caCertificates []*CaCertificate) {
	w := tabwriter.NewWriter(os.Stdout, 10, 4, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "CERTIFICATE\tEXPIRES\tRESIDUAL TIME\tCERTIFICATE AUTHORITY\tNODE")
	for _, cert := range certificates {
		s := fmt.Sprintf("%s\t%s\t%s\t%s\t%-8v",
			cert.Name,
			cert.Expires,
			cert.Residual,
			cert.AuthorityName,
			cert.NodeName,
		)

		_, _ = fmt.Fprintln(w, s)
		continue
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "CERTIFICATE AUTHORITY\tEXPIRES\tRESIDUAL TIME\tNODE")
	for _, caCert := range caCertificates {
		c := fmt.Sprintf("%s\t%s\t%s\t%-8v",
			caCert.AuthorityName,
			caCert.Expires,
			caCert.Residual,
			caCert.NodeName,
		)

		_, _ = fmt.Fprintln(w, c)
		continue
	}

	_ = w.Flush()
}
func getCertInfo(certContext, certFileName, nodeName string) (*Certificate, error) {
	certs, err1 := certutil.ParseCertsPEM([]byte(certContext))
	if err1 != nil {
		return nil, errors.Wrap(err1, "Failed to get cluster certs")
	}
	var authorityName string
	switch certFileName {
	case "apiserver.crt":
		authorityName = "ca"
	case "apiserver-kubelet-client.crt":
		authorityName = "ca"
	case "front-proxy-client.crt":
		authorityName = "front-proxy-ca"
	default:
		authorityName = ""
	}
	cert := Certificate{
		Name:          certFileName,
		Expires:       certs[0].NotAfter.Format("Jan 02, 2006 15:04 MST"),
		Residual:      ResidualTime(certs[0].NotAfter),
		AuthorityName: authorityName,
		NodeName:      nodeName,
	}
	return &cert, nil
}
func getCacertInfo(certContext, certFileName, nodeName string) (*CaCertificate, error) {
	certs, err1 := certutil.ParseCertsPEM([]byte(certContext))
	if err1 != nil {
		return nil, errors.Wrap(err1, "Failed to get cluster certs")
	}
	cert1 := CaCertificate{
		AuthorityName: certFileName,
		Expires:       certs[0].NotAfter.Format("Jan 02, 2006 15:04 MST"),
		Residual:      ResidualTime(certs[0].NotAfter),
		NodeName:      nodeName,
	}
	return &cert1, nil
}
func ResidualTime(t time.Time) string {
	d := time.Until(t)
	if seconds := int(d.Seconds()); seconds < -1 {
		return fmt.Sprintf("<invalid>")
	} else if seconds < 0 {
		return fmt.Sprintf("0s")
	} else if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if minutes := int(d.Minutes()); minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	} else if hours := int(d.Hours()); hours < 24 {
		return fmt.Sprintf("%dh", hours)
	} else if hours < 24*365 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}
func RenewClusterCert(m *manager.Manager) error {
	m.Logger.Infoln("Renewing cluster certs ...")
	return m.RunTaskOnMasterNodes(renewClusterCerts, false)
}
func SyncKubeConfig(m *manager.Manager) error {
	m.Logger.Infoln("Syncing cluster kubeConfig ...")
	return m.RunTaskOnWorkerNodes(syncKubeConfig, true)
}

func renewClusterCerts(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", strings.Join(kubeadmList, " && ")), 5, false)
	if err != nil {
		return errors.Wrap(err, "Failed to kubeadm alpha certs renew...")
	}
	_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", strings.Join(restartList, " && ")), 5, false)
	if err1 != nil {
		return errors.Wrap(err1, "Failed to restart kube-apiserver or kube-schedule or kube-controller-manager")
	}

	if err := kubernetes.GetKubeConfig(mgr); err != nil {
		return err
	}
	if mgr.Runner.Index == 0 {
		kubeCfgBase64Cmd := "cat /etc/kubernetes/admin.conf | base64 --wrap=0"
		kubeConfigStr, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", kubeCfgBase64Cmd), 1, false)
		if err1 != nil {
			return errors.Wrap(errors.WithStack(err1), "Failed to get cluster kubeconfig")
		}
		kubeConfigValue["kubeConfig"] = kubeConfigStr
	}
	return nil
}
func syncKubeConfig(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	createConfigDirCmd := "mkdir -p /root/.kube && mkdir -p $HOME/.kube"
	chownKubeConfig := "chown $(id -u):$(id -g) -R $HOME/.kube"
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", createConfigDirCmd), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create kube dir")
	}
	syncKubeconfigForRootCmd := fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > %s\"", kubeConfigValue["kubeConfig"], "/root/.kube/config")
	syncKubeconfigForUserCmd := fmt.Sprintf("echo %s | base64 -d > %s && %s", kubeConfigValue["kubeConfig"], "$HOME/.kube/config", chownKubeConfig)
	if _, err := mgr.Runner.ExecuteCmd(syncKubeconfigForRootCmd, 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to sync kube config")
	}
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", syncKubeconfigForUserCmd), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to sync kube config")
	}
	return nil
}
