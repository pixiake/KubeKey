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

package v1beta2

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"regexp"
	"strings"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
)

var (
	funcMap = template.FuncMap{"toYaml": toYAML, "indent": Indent}
	// KubeadmCfgTempl defines the template of kubeadm configuration file.
	KubeadmCfgTempl = template.Must(template.New("kubeadmCfg").Funcs(funcMap).Parse(
		dedent.Dedent(`---
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
etcd:
  external:
    endpoints:
    {{- range .ExternalEtcd.Endpoints }}
    - {{ . }}
    {{- end }}
    caFile: {{ .ExternalEtcd.CaFile }}
    certFile: {{ .ExternalEtcd.CertFile }}
    keyFile: {{ .ExternalEtcd.KeyFile }}
dns:
  type: CoreDNS
  imageRepository: {{ .CorednsRepo }}
  imageTag: {{ .CorednsTag }}
imageRepository: {{ .ImageRepo }}
kubernetesVersion: {{ .Version }}
certificatesDir: /etc/kubernetes/pki
clusterName: {{ .ClusterName }}
controlPlaneEndpoint: {{ .ControlPlaneEndpoint }}
networking:
  dnsDomain: {{ .ClusterName }}
  podSubnet: {{ .PodSubnet }}
  serviceSubnet: {{ .ServiceSubnet }}
apiServer:
  extraArgs:
{{ toYaml .ApiServerArgs | indent 4}}
  certSANs:
    {{- range .CertSANs }}
    - {{ . }}
    {{- end }}
controllerManager:
  extraArgs:
    node-cidr-mask-size: "{{ .NodeCidrMaskSize }}"
{{ toYaml .ControllerManagerArgs | indent 4 }}
  extraVolumes:
  - name: host-time
    hostPath: /etc/localtime
    mountPath: /etc/localtime
    readOnly: true
scheduler:
  extraArgs:
{{ toYaml .SchedulerArgs | indent 4 }}

---
apiVersion: kubeadm.k8s.io/v1beta2
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: {{ .ControlPlanAddr }}
  bindPort: {{ .ControlPlanPort }}
{{- if .CriSock }}
nodeRegistration:
  criSocket: {{ .CriSock }}
{{- end }}
---
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
{{ toYaml .KubeProxyConfiguration }}
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
{{ toYaml .KubeletConfiguration }}

    `)))
)

var (
	apiServerArgs = map[string]string{
		"bind-address":        "0.0.0.0",
		"audit-log-maxage":    "30",
		"audit-log-maxbackup": "10",
		"audit-log-maxsize":   "100",
		"audit-log-path":      "/var/log/apiserver/audit.log",
		"feature-gates":       "CSINodeInfo=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true,RotateKubeletServerCertificate=true",
	}
	controllermanagerArgs = map[string]string{
		"bind-address":                          "0.0.0.0",
		"experimental-cluster-signing-duration": "87600h",
		"feature-gates":                         "CSINodeInfo=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true,RotateKubeletServerCertificate=true",
	}
	schedulerArgs = map[string]string{
		"bind-address":  "0.0.0.0",
		"feature-gates": "CSINodeInfo=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true,RotateKubeletClientCertificate=true,RotateKubeletServerCertificate=true",
	}
)

// GenerateKubeadmCfg create kubeadm configuration file to initialize the cluster.
func GenerateKubeadmCfg(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) (string, error) {
	// generate etcd configuration
	var externalEtcd kubekeyapiv1alpha1.ExternalEtcd
	var endpointsList []string
	var caFile, certFile, keyFile, containerRuntimeEndpoint string

	for _, host := range mgr.EtcdNodes {
		endpoint := fmt.Sprintf("https://%s:%s", host.InternalAddress, kubekeyapiv1alpha1.DefaultEtcdPort)
		endpointsList = append(endpointsList, endpoint)
	}
	externalEtcd.Endpoints = endpointsList

	caFile = "/etc/ssl/etcd/ssl/ca.pem"
	certFile = fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s.pem", mgr.MasterNodes[0].Name)
	keyFile = fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s-key.pem", mgr.MasterNodes[0].Name)

	externalEtcd.CaFile = caFile
	externalEtcd.CertFile = certFile
	externalEtcd.KeyFile = keyFile

	_, ApiServerArgs := util.GetArgs(apiServerArgs, mgr.Cluster.Kubernetes.ApiServerArgs)
	_, ControllerManagerArgs := util.GetArgs(controllermanagerArgs, mgr.Cluster.Kubernetes.ControllerManagerArgs)
	_, SchedulerArgs := util.GetArgs(schedulerArgs, mgr.Cluster.Kubernetes.SchedulerArgs)

	// generate cri configuration
	switch mgr.Cluster.Kubernetes.ContainerManager {
	case "docker":
		containerRuntimeEndpoint = ""
	case "crio":
		containerRuntimeEndpoint = kubekeyapiv1alpha1.DefaultCrioEndpoint
	case "containerd":
		containerRuntimeEndpoint = kubekeyapiv1alpha1.DefaultContainerdEndpoint
	case "isula":
		containerRuntimeEndpoint = kubekeyapiv1alpha1.DefaultIsulaEndpoint
	default:
		containerRuntimeEndpoint = ""
	}

	if mgr.Cluster.Kubernetes.ContainerRuntimeEndpoint != "" {
		containerRuntimeEndpoint = mgr.Cluster.Kubernetes.ContainerRuntimeEndpoint
	}

	return util.Render(KubeadmCfgTempl, util.Data{
		"ImageRepo":              strings.TrimSuffix(preinstall.GetImage(mgr, "kube-apiserver").ImageRepo(), "/kube-apiserver"),
		"CorednsRepo":            strings.TrimSuffix(preinstall.GetImage(mgr, "coredns").ImageRepo(), "/coredns"),
		"CorednsTag":             preinstall.GetImage(mgr, "coredns").Tag,
		"Version":                mgr.Cluster.Kubernetes.Version,
		"ClusterName":            mgr.Cluster.Kubernetes.ClusterName,
		"ControlPlanAddr":        mgr.Cluster.ControlPlaneEndpoint.Address,
		"ControlPlanPort":        mgr.Cluster.ControlPlaneEndpoint.Port,
		"ControlPlaneEndpoint":   fmt.Sprintf("%s:%d", mgr.Cluster.ControlPlaneEndpoint.Domain, mgr.Cluster.ControlPlaneEndpoint.Port),
		"PodSubnet":              mgr.Cluster.Network.KubePodsCIDR,
		"ServiceSubnet":          mgr.Cluster.Network.KubeServiceCIDR,
		"CertSANs":               mgr.Cluster.GenerateCertSANs(),
		"ExternalEtcd":           externalEtcd,
		"NodeCidrMaskSize":       mgr.Cluster.Kubernetes.NodeCidrMaskSize,
		"CriSock":                containerRuntimeEndpoint,
		"InternalLBDisabled":     !mgr.Cluster.ControlPlaneEndpoint.IsInternalLBEnabled(),
		"AdvertiseAddress":       node.InternalAddress,
		"ApiServerArgs":          ApiServerArgs,
		"ControllerManagerArgs":  ControllerManagerArgs,
		"SchedulerArgs":          SchedulerArgs,
		"KubeletConfiguration":   GetKubeletConfiguration(mgr, containerRuntimeEndpoint),
		"KubeProxyConfiguration": getKubeProxyConfiguration(mgr),
	})
}

func getKubeletCgroupDriver(mgr *manager.Manager) (string, error) {
	var cmd, kubeletCgroupDriver string
	switch mgr.Cluster.Kubernetes.ContainerManager {
	case "docker":
		cmd = "docker info | grep 'Cgroup Driver' | awk -F': ' '{ print $2; }'"
	case "crio":
		cmd = "crio config | grep cgroup_manager | awk -F'= ' '{ print $2; }'"
	case "containerd":
		cmd = "containerd config dump | grep systemd_cgroup | awk -F'= ' '{ print $2; }'"
	case "isula":
		cmd = "isula info | grep 'Cgroup Driver' | awk -F': ' '{ print $2; }'"
	default:
		kubeletCgroupDriver = ""
	}

	checkResult, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo env PATH=$PATH /bin/sh -c \"%s\"", cmd), 3, false)
	if err != nil {
		return "", errors.Wrap(errors.WithStack(err), "Failed to get container runtime cgroup driver.")
	}
	if strings.Contains(checkResult, "systemd") && !strings.Contains(checkResult, "false") {
		kubeletCgroupDriver = "systemd"
	} else {
		kubeletCgroupDriver = ""
	}

	return kubeletCgroupDriver, nil
}

func GetKubeletConfiguration(mgr *manager.Manager, criSock string) map[string]interface{} {
	defaultKubeletConfiguration := map[string]interface{}{
		"clusterDomain":      mgr.Cluster.Kubernetes.ClusterName,
		"clusterDNS":         []string{"169.254.25.10"},
		"maxPods":            mgr.Cluster.Kubernetes.MaxPods,
		"rotateCertificates": true,
		"kubeReserved": map[string]string{
			"cpu":    "200m",
			"memory": "250Mi",
		},
		"systemReserved": map[string]string{
			"cpu":    "200m",
			"memory": "250Mi",
		},
		"evictionHard": map[string]string{
			"memory.available": "5%",
		},
		"evictionSoft": map[string]string{
			"memory.available": "10%",
		},
		"evictionSoftGracePeriod": map[string]string{
			"memory.available": "2m",
		},
		"evictionMaxPodGracePeriod":        120,
		"evictionPressureTransitionPeriod": "30s",
		"featureGates": map[string]bool{
			"CSINodeInfo":                    true,
			"VolumeSnapshotDataSource":       true,
			"ExpandCSIVolumes":               true,
			"RotateKubeletClientCertificate": true,
			"RotateKubeletServerCertificate": true,
		},
	}

	cgroupDriver, err := getKubeletCgroupDriver(mgr)
	if err != nil {
		logrus.Fatal(err)
	}
	if len(cgroupDriver) != 0 {
		defaultKubeletConfiguration["cgroupDriver"] = "systemd"
	}

	if len(criSock) != 0 {
		defaultKubeletConfiguration["containerLogMaxSize"] = "5Mi"
		defaultKubeletConfiguration["containerLogMaxFiles"] = 3
	}

	customKubeletConfiguration := make(map[string]interface{})
	if len(mgr.Cluster.Kubernetes.KubeletConfiguration.Raw) != 0 {
		err := yaml.Unmarshal(mgr.Cluster.Kubernetes.KubeletConfiguration.Raw, &customKubeletConfiguration)
		if err != nil {
			logrus.Fatal("failed to parse kubelet's configuration")
		}
	}

	kubeletConfiguration := make(map[string]interface{})
	if len(customKubeletConfiguration) != 0 {
		for customArg := range customKubeletConfiguration {
			if _, ok := defaultKubeletConfiguration[customArg]; ok {
				kubeletConfiguration[customArg] = customKubeletConfiguration[customArg]
				delete(defaultKubeletConfiguration, customArg)
				delete(customKubeletConfiguration, customArg)
			} else {
				kubeletConfiguration[customArg] = customKubeletConfiguration[customArg]
			}
		}
	}

	if len(defaultKubeletConfiguration) != 0 {
		for defaultArg := range defaultKubeletConfiguration {
			kubeletConfiguration[defaultArg] = defaultKubeletConfiguration[defaultArg]
		}
	}
	return kubeletConfiguration
}

func getKubeProxyConfiguration(mgr *manager.Manager) map[string]interface{} {
	defaultKubeProxyConfiguration := map[string]interface{}{
		"clusterCIDR": mgr.Cluster.Network.KubePodsCIDR,
		"mode":        mgr.Cluster.Kubernetes.ProxyMode,
		"iptables": map[string]interface{}{
			"masqueradeAll": mgr.Cluster.Kubernetes.MasqueradeAll,
			"masqueradeBit": 14,
			"minSyncPeriod": "0s",
			"syncPeriod":    "30s",
		},
	}

	customKubeProxyConfiguration := make(map[string]interface{})
	if len(mgr.Cluster.Kubernetes.KubeProxyConfiguration.Raw) != 0 {
		err := yaml.Unmarshal(mgr.Cluster.Kubernetes.KubeProxyConfiguration.Raw, &customKubeProxyConfiguration)
		if err != nil {
			logrus.Fatal("failed to parse kube-proxy's configuration")
		}
	}

	kubeProxyConfiguration := make(map[string]interface{})
	if len(customKubeProxyConfiguration) != 0 {
		for customArg := range customKubeProxyConfiguration {
			if _, ok := defaultKubeProxyConfiguration[customArg]; ok {
				kubeProxyConfiguration[customArg] = customKubeProxyConfiguration[customArg]
				delete(defaultKubeProxyConfiguration, customArg)
				delete(customKubeProxyConfiguration, customArg)
			} else {
				kubeProxyConfiguration[customArg] = customKubeProxyConfiguration[customArg]
			}
		}
	}

	if len(defaultKubeProxyConfiguration) != 0 {
		for defaultArg := range defaultKubeProxyConfiguration {
			kubeProxyConfiguration[defaultArg] = defaultKubeProxyConfiguration[defaultArg]
		}
	}

	return kubeProxyConfiguration
}

func toYAML(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

func Indent(n int, text string) string {
	startOfLine := regexp.MustCompile(`(?m)^`)
	indentation := strings.Repeat(" ", n)
	return startOfLine.ReplaceAllLiteralString(text, indentation)
}
