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

package config

import (
	"fmt"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"strings"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
)

var (
	// K3sServiceTempl defines the template of kubelete service for systemd.
	K3sServiceTempl = template.Must(template.New("k3sService").Parse(
		dedent.Dedent(`[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
Wants=network-online.target
After=network-online.target

[Install]
WantedBy=multi-user.target

[Service]
Type=notify
EnvironmentFile=/etc/systemd/system/k3s.service.env
{{ if .IsMaster }}
Environment="K3S_ARGS= {{ range .CertSANs }} --tls-san={{ . }}{{- end }} {{ range .ApiserverArgs }} --kube-apiserver-arg={{ . }}{{- end }} {{ range .ControllerManager }} --kube-controller-manager-arg={{ . }}{{- end }} {{ range .SchedulerArgs }} --kube-scheduler-arg={{ . }}{{- end }} --cluster-cidr={{ .PodSubnet }} --service-cidr={{ .ServiceSubnet }} --cluster-dns={{ .ClusterDns }} --flannel-backend=none --disable-network-policy --disable-cloud-controller --disable=servicelb,traefik,metrics-server,local-storage"
{{ end }}
Environment="K3S_EXTRA_ARGS=--node-name={{ .HostName }}  --node-ip={{ .NodeIP }}  --pause-image={{ .PauseImage }} {{ range .KubeletArgs }} --kubelet-arg={{ . }}{{- end }} {{ range .KubeProxyArgs }} --kube-proxy-arg={{ . }}{{- end }}"
Environment="K3S_ROLE={{ if .IsMaster }}server{{ else }}agent{{ end }}"
Environment="K3S_SERVER_ARGS={{ if .Server }}--server={{ .Server }}{{ end }}"
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s $K3S_ROLE $K3S_ARGS $K3S_EXTRA_ARGS $K3S_SERVER_ARGS
    `)))

	// K3sEnvTempl defines the template of kubelet's Env for the kubelet's systemd service.
	K3sEnvTempl = template.Must(template.New("k3sEnv").Parse(
		dedent.Dedent(`# Note: This dropin only works with k3s
{{ if .IsMaster }}
K3S_DATASTORE_ENDPOINT={{ .DataStoreEndPoint }}
K3S_DATASTORE_CAFILE={{ .DataStoreCaFile }}
K3S_DATASTORE_CERTFILE={{ .DataStoreCertFile }}
K3S_DATASTORE_KEYFILE={{ .DataStoreKeyFile }}
K3S_KUBECONFIG_MODE=644
{{ end }}
{{ if .Token }}
K3S_TOKEN={{ .Token }}
{{ end }}

    `)))
)

// GenerateK3sService is used to generate kubelet's service content for systemd.
func GenerateK3sService(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg, token string) (string, error) {
	var server string
	if token != "" {
		server = fmt.Sprintf("https://%s:%d", mgr.Cluster.ControlPlaneEndpoint.Domain, mgr.Cluster.ControlPlaneEndpoint.Port)
	} else {
		server = ""
	}

	defaultKubeletArs := map[string]string{
		"cni-conf-dir":    "/etc/cni/net.d",
		"cni-bin-dir":     "/opt/cni/bin",
		"kube-reserved":   "cpu=200m,memory=250Mi,ephemeral-storage=1Gi",
		"system-reserved": "cpu=200m,memory=250Mi,ephemeral-storage=1Gi",
		"eviction-hard":   "memory.available<5%,nodefs.available<10%",
	}
	defaultKubeProxyArgs := map[string]string{
		"proxy-mode": "ipvs",
	}

	kubeApiserverArgs, _ := util.GetArgs(map[string]string{}, mgr.Cluster.Kubernetes.ApiServerArgs)
	kubeControllerManager, _ := util.GetArgs(map[string]string{
		"pod-eviction-timeout":        "3m0s",
		"terminated-pod-gc-threshold": "5",
	}, mgr.Cluster.Kubernetes.ControllerManagerArgs)
	kubeSchedulerArgs, _ := util.GetArgs(map[string]string{}, mgr.Cluster.Kubernetes.SchedulerArgs)
	kubeletArgs, _ := util.GetArgs(defaultKubeletArs, mgr.Cluster.Kubernetes.KubeletArgs)
	kubeProxyArgs, _ := util.GetArgs(defaultKubeProxyArgs, mgr.Cluster.Kubernetes.KubeProxyArgs)

	return util.Render(K3sServiceTempl, util.Data{
		"Server":            server,
		"IsMaster":          node.IsMaster,
		"NodeIP":            node.InternalAddress,
		"HostName":          node.Name,
		"PodSubnet":         mgr.Cluster.Network.KubePodsCIDR,
		"ServiceSubnet":     mgr.Cluster.Network.KubeServiceCIDR,
		"ClusterDns":        mgr.Cluster.ClusterIP(),
		"CertSANs":          mgr.Cluster.GenerateCertSANs(),
		"PauseImage":        preinstall.GetImage(mgr, "pause").ImageName(),
		"ApiserverArgs":     kubeApiserverArgs,
		"ControllerManager": kubeControllerManager,
		"SchedulerArgs":     kubeSchedulerArgs,
		"KubeletArgs":       kubeletArgs,
		"KubeProxyArgs":     kubeProxyArgs,
	})
}

// GenerateK3sEnv is used to generate the env content of kubelet's service for systemd.
func GenerateK3sEnv(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg, token string) (string, error) {
	// generate etcd configuration
	var externalEtcd kubekeyapiv1alpha1.ExternalEtcd
	var endpointsList []string
	var caFile, certFile, keyFile string

	for _, host := range mgr.EtcdNodes {
		endpoint := fmt.Sprintf("https://%s:%s", host.InternalAddress, kubekeyapiv1alpha1.DefaultEtcdPort)
		endpointsList = append(endpointsList, endpoint)
	}
	externalEtcd.Endpoints = endpointsList

	externalEtcdEndpoints := strings.Join(endpointsList, ",")
	caFile = "/etc/ssl/etcd/ssl/ca.pem"
	certFile = fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s.pem", mgr.MasterNodes[0].Name)
	keyFile = fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s-key.pem", mgr.MasterNodes[0].Name)

	externalEtcd.CaFile = caFile
	externalEtcd.CertFile = certFile
	externalEtcd.KeyFile = keyFile

	return util.Render(K3sEnvTempl, util.Data{
		"DataStoreEndPoint": externalEtcdEndpoints,
		"DataStoreCaFile":   caFile,
		"DataStoreCertFile": certFile,
		"DataStoreKeyFile":  keyFile,
		"IsMaster":          node.IsMaster,
		"Token":             token,
	})
}
