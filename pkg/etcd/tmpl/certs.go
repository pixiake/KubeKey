/*
Copyright 2020 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tmpl

import (
	"strings"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
)

// Add is used in the template to implement the addition operation.
func Add(a int, b int) int {
	return a + b
}

var (
	funcMap = template.FuncMap{"Add": Add}
	// EtcdSslCfgTempl defines the template of openssl's configuration for etcd.
	EtcdSslCfgTempl = template.Must(template.New("etcdSslCfg").Funcs(funcMap).Parse(
		dedent.Dedent(`[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[ ssl_client ]
extendedKeyUsage = clientAuth, serverAuth
basicConstraints = CA:FALSE
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
subjectAltName = @alt_names

[ v3_ca ]
basicConstraints = CA:TRUE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names
authorityKeyIdentifier=keyid:always,issuer

[alt_names]
{{- range $i, $v := .Dns }}
DNS.{{ Add $i 1 }} = {{ $v }}
{{- end }}
{{- range $i, $v := .Ips }}
IP.{{ Add $i 1 }} = {{ $v }}
{{- end }}

    `)))

	// EtcdSslTempl defines the template of the script for generating etcd certs.
	EtcdSslTempl = template.Must(template.New("etcdSsl").Parse(
		dedent.Dedent(`#!/bin/bash

# Author: Smana smainklh@gmail.com
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o pipefail
usage()
{
    cat << EOF
Create self signed certificates

Usage : $(basename $0) -f <config> [-d <ssldir>]
      -h | --help         : Show this message
      -f | --config       : Openssl configuration file
      -d | --ssldir       : Directory where the certificates will be installed

               ex :
               $(basename $0) -f openssl.conf -d /srv/ssl
EOF
}

# Options parsing
while (($#)); do
    case "$1" in
        -h | --help)   usage;   exit 0;;
        -f | --config) CONFIG=${2}; shift 2;;
        -d | --ssldir) SSLDIR="${2}"; shift 2;;
        *)
            usage
            echo "ERROR : Unknown option"
            exit 3
        ;;
    esac
done

if [ -z ${CONFIG} ]; then
    echo "ERROR: the openssl configuration file is missing. option -f"
    exit 1
fi
if [ -z ${SSLDIR} ]; then
    SSLDIR="/etc/ssl/etcd"
fi

tmpdir=$(mktemp -d /tmp/etcd_cacert.XXXXXX)
trap 'rm -rf "${tmpdir}"' EXIT
cd "${tmpdir}"

mkdir -p "${SSLDIR}"

# Root CA
if [ -e "$SSLDIR/ca-key.pem" ]; then
    # Reuse existing CA
    cp $SSLDIR/{ca.pem,ca-key.pem} .
else
    openssl genrsa -out ca-key.pem 2048 > /dev/null 2>&1
    openssl req -x509 -new -nodes -key ca-key.pem -days 36500 -out ca.pem -subj "/CN=etcd-ca" > /dev/null 2>&1
fi

MASTERS='{{ .Masters }}'
HOSTS='{{ .Hosts }}'

# ETCD member
if [ -n "$MASTERS" ]; then
    for host in $MASTERS; do
        cn="${host%%.*}"
        # Member key
        openssl genrsa -out member-${host}-key.pem 2048 > /dev/null 2>&1
        openssl req -new -key member-${host}-key.pem -out member-${host}.csr -subj "/CN=etcd-member-${cn}" -config ${CONFIG} > /dev/null 2>&1
        openssl x509 -req -in member-${host}.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial -out member-${host}.pem -days 36500 -extensions ssl_client -extfile ${CONFIG} > /dev/null 2>&1

        # Admin key
        openssl genrsa -out admin-${host}-key.pem 2048 > /dev/null 2>&1
        openssl req -new -key admin-${host}-key.pem -out admin-${host}.csr -subj "/CN=etcd-admin-${cn}" > /dev/null 2>&1
        openssl x509 -req -in admin-${host}.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial -out admin-${host}.pem -days 36500 -extensions ssl_client  -extfile ${CONFIG} > /dev/null 2>&1
    done
fi

# Node keys
if [ -n "$HOSTS" ]; then
    for host in $HOSTS; do
        cn="${host%%.*}"
        openssl genrsa -out node-${host}-key.pem 2048 > /dev/null 2>&1
        openssl req -new -key node-${host}-key.pem -out node-${host}.csr -subj "/CN=etcd-node-${cn}" > /dev/null 2>&1
        openssl x509 -req -in node-${host}.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial -out node-${host}.pem -days 36500 -extensions ssl_client  -extfile ${CONFIG} > /dev/null 2>&1
    done
fi

# Install certs
if [ -e "$SSLDIR/ca-key.pem" ]; then
    # No pass existing CA
    rm -f ca.pem ca-key.pem
fi

mv *.pem ${SSLDIR}/
    `)))
)

// GenerateEtcdSslCfg is used to generate openssl configuration content for etcd.
func GenerateEtcdSslCfg(cfg *kubekeyapiv1alpha1.ClusterSpec) (string, error) {
	dnsList := []string{"localhost", "etcd.kube-system.svc.cluster.local", "etcd.kube-system.svc", "etcd.kube-system", "etcd"}
	ipList := []string{"127.0.0.1"}

	if cfg.ControlPlaneEndpoint.Domain == "" {
		dnsList = append(dnsList, kubekeyapiv1alpha1.DefaultLBDomain)
	} else {
		dnsList = append(dnsList, cfg.ControlPlaneEndpoint.Domain)
	}

	for _, host := range cfg.Hosts {
		dnsList = append(dnsList, host.Name)
		ipList = append(ipList, host.InternalAddress)
	}

	return util.Render(EtcdSslCfgTempl, util.Data{
		"Dns": dnsList,
		"Ips": ipList,
	})
}

// GenerateEtcdSslScript is used to generate the script content for generating etcd certs.
func GenerateEtcdSslScript(mgr *manager.Manager) (string, error) {
	var masters []string
	var hosts []string

	for _, host := range mgr.EtcdNodes {
		masters = append(masters, host.Name)
	}
	for _, host := range mgr.MasterNodes {
		hosts = append(hosts, host.Name)
	}
	return util.Render(EtcdSslTempl, util.Data{
		"Masters": strings.Join(masters, " "),
		"Hosts":   strings.Join(hosts, " "),
	})
}
