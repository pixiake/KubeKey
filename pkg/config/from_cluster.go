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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// ClusterCfgTempl defines the template of cluster configuration file for the existing cluster.
	ClusterCfgTempl = template.Must(template.New("ClusterCfg").Parse(
		dedent.Dedent(`apiVersion: kubekey.kubesphere.io/v1alpha1
kind: Cluster
metadata:
  name: {{ .Options.Name }}
spec:
  hosts: 
  ##You should complete the ssh information of the hosts
  {{- range .Options.Hosts }}
  - {{ . }}
  {{- end }}
  roleGroups:
    etcd:
    - SHOULD_BE_REPLACED
    master: 
    {{- range .Options.MasterGroup }}
    - {{ . }}
    {{- end }}
    worker:
    {{- range .Options.WorkerGroup }}
    - {{ . }}
    {{- end }}
  controlPlaneEndpoint:
    ##Internal loadbalancer for apiservers 
    {{- if .Options.InternalLoadbalancer }}
    internalLoadbalancer: {{ .Options.InternalLoadbalancer }}
    {{- else }}
    #internalLoadbalancer: haproxy
    {{- end }}

    ##If the external loadbalancer was used, 'address' should be set to loadbalancer's ip.
    domain: {{ .Options.ControlPlaneEndpointDomain }}
    address: {{ .Options.ControlPlaneEndpointAddress }}
    port: {{ .Options.ControlPlaneEndpointPort }}
  kubernetes:
    version: {{ .Options.KubeVersion }}
    clusterName: {{ .Options.ClusterName }}
    proxyMode: {{ .Options.ProxyMode }}
    masqueradeAll: {{ .Options.MasqueradeAll }}
    maxPods: {{ .Options.MaxPods }}
    nodeCidrMaskSize: {{ .Options.NodeCidrMaskSize }}
  network:
    plugin: {{ .Options.NetworkPlugin }}
    kubePodsCIDR: {{ .Options.PodNetworkCidr }}
    kubeServiceCIDR: {{ .Options.ServiceNetworkCidr }}
  registry:
    privateRegistry: ""

    `)))
)

// GenerateClusterCfgStr is used to generate cluster configuration content.
func GenerateClusterCfgStr(opt *OptionsCluster) (string, error) {
	return util.Render(ClusterCfgTempl, util.Data{
		"Options": opt,
	})
}

// OptionsCluster defineds the parameters of cluster configuration for the existing cluster.
type OptionsCluster struct {
	Name                        string
	Hosts                       []string
	MasterGroup                 []string
	WorkerGroup                 []string
	KubeVersion                 string
	ImageRepo                   string
	ClusterName                 string
	MasqueradeAll               string
	ProxyMode                   string
	MaxPods                     string
	NodeCidrMaskSize            string
	PodNetworkCidr              string
	ServiceNetworkCidr          string
	NetworkPlugin               string
	ControlPlaneEndpointDomain  string
	ControlPlaneEndpointAddress string
	ControlPlaneEndpointPort    string
	InternalLoadbalancer        string
}

// GetInfoFromCluster is used to fetch information from the existing cluster.
func GetInfoFromCluster(config, name string) (*OptionsCluster, error) {
	clientset, err := util.NewClient(config)
	if err != nil {
		return nil, err
	}

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	opt := OptionsCluster{}
	if name != "" {
		output := strings.Split(name, ".")
		opt.Name = output[0]
	} else {
		opt.Name = "config-sample"
	}

	for _, node := range nodes.Items {
		nodeCfg := kubekeyapiv1alpha1.HostCfg{}
		nodeInfo, err := clientset.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		for _, address := range nodeInfo.Status.Addresses {
			if address.Type == "Hostname" {
				nodeCfg.Name = address.Address
			}
			if address.Type == "InternalIP" {
				nodeCfg.Address = address.Address
				nodeCfg.InternalAddress = address.Address
			}
		}
		if _, ok := nodeInfo.Labels["node-role.kubernetes.io/master"]; ok {
			opt.MasterGroup = append(opt.MasterGroup, nodeCfg.Name)
			if _, ok := nodeInfo.Labels["node-role.kubernetes.io/worker"]; ok {
				opt.WorkerGroup = append(opt.WorkerGroup, nodeCfg.Name)
			}
		} else {
			opt.WorkerGroup = append(opt.WorkerGroup, nodeCfg.Name)
		}
		nodeCfgStr := fmt.Sprintf("{name: %s, address: %s, internalAddress: %s}", nodeCfg.Name, nodeCfg.Address, nodeCfg.InternalAddress)
		opt.Hosts = append(opt.Hosts, nodeCfgStr)

		opt.MaxPods = nodeInfo.Status.Capacity.Pods().String()
	}

	kubeadmConfig, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "kubeadm-config", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBuffer([]byte(kubeadmConfig.Data["ClusterConfiguration"]))); err != nil {
		return nil, err
	}
	opt.KubeVersion = viper.GetString("kubernetesVersion")
	opt.ImageRepo = viper.GetString("imageRepository")
	opt.ClusterName = viper.GetString("clusterName")
	opt.PodNetworkCidr = viper.GetString("networking.podSubnet")
	opt.ServiceNetworkCidr = viper.GetString("networking.serviceSubnet")
	if viper.IsSet("controllerManager.extraArgs.node-cidr-mask-size") {
		opt.NodeCidrMaskSize = viper.GetString("controllerManager.extraArgs.node-cidr-mask-size")
	} else {
		opt.NodeCidrMaskSize = "24"
	}
	if viper.IsSet("controlPlaneEndpoint") {
		controlPlaneEndpointStr := viper.GetString("controlPlaneEndpoint")
		strList := strings.Split(controlPlaneEndpointStr, ":")
		opt.ControlPlaneEndpointPort = strList[len(strList)-1]
		strList = strList[:len(strList)-1]
		address := strings.Join(strList, ":")
		ip := net.ParseIP(address)
		if ip != nil {
			opt.ControlPlaneEndpointAddress = address
			opt.ControlPlaneEndpointDomain = "lb.kubesphere.local"
		} else {
			opt.ControlPlaneEndpointAddress = "\"\""
			opt.ControlPlaneEndpointDomain = address
		}
	}

	pods, err := clientset.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "calico") {
			opt.NetworkPlugin = "calico"
		}
		if strings.Contains(pod.Name, "flannel") {
			opt.NetworkPlugin = "flannel"
		}
		if strings.Contains(pod.Name, "haproxy-node") {
			opt.InternalLoadbalancer = "haproxy"
		}
	}

	kubeProxyConfig, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "kube-proxy", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBuffer([]byte(kubeProxyConfig.Data["config.conf"]))); err != nil {
		return nil, err
	}

	opt.MasqueradeAll = viper.GetString("iptables.masqueradeAll")
	if viper.GetString("mode") == "ipvs" {
		opt.ProxyMode = viper.GetString("mode")
	} else {
		opt.ProxyMode = "iptables"
	}

	return &opt, nil
}

// GenerateConfigFromCluster is used to generate cluster configuration file from the existing cluster's information.
func GenerateConfigFromCluster(cfgPath, kubeconfig, name string) error {
	opt, err := GetInfoFromCluster(kubeconfig, name)
	if err != nil {
		return err
	}

	ClusterCfgStr, err := GenerateClusterCfgStr(opt)
	if err != nil {
		return errors.Wrap(err, "Failed to generate cluster config")
	}
	ClusterCfgStrBase64 := base64.StdEncoding.EncodeToString([]byte(ClusterCfgStr))
	var configPath string

	if cfgPath != "" {
		CheckConfigFileStatus(cfgPath)
		cmd := fmt.Sprintf("echo %s | base64 -d > %s", ClusterCfgStrBase64, cfgPath)
		if output, err := exec.Command("/bin/sh", "-c", cmd).Output(); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to write config to %s: %s", cfgPath, strings.TrimSpace(string(output))))
		}
		configPath = cfgPath
	} else {
		currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return errors.Wrap(err, "Failed to get current dir")
		}
		configPath = fmt.Sprintf("%s/%s.yaml", currentDir, opt.Name)
		CheckConfigFileStatus(configPath)
		cmd := fmt.Sprintf("echo %s | base64 -d > %s/%s.yaml", ClusterCfgStrBase64, currentDir, opt.Name)
		err1 := exec.Command("/bin/sh", "-c", cmd).Run()
		if err1 != nil {
			return err1
		}

	}
	notice := "Notice: " + fmt.Sprintf("%s has been created. Some parameters need to be filled in by yourself, please complete it.", configPath)
	fmt.Printf("\033[1;36m%s\033[0m\n\n", notice)
	return nil
}
