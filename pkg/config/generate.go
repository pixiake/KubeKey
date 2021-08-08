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
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/kubesphere"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
)

var (
	// ClusterObjTempl defines the template of cluster configuration file default.
	ClusterObjTempl = template.Must(template.New("Cluster").Parse(
		dedent.Dedent(`apiVersion: kubekey.kubesphere.io/v1alpha1
kind: Cluster
metadata:
  name: {{ .Options.Name }}
spec:
  hosts:
  - {name: node1, address: 172.16.0.2, internalAddress: 172.16.0.2, user: ubuntu, password: Qcloud@123}
  - {name: node2, address: 172.16.0.3, internalAddress: 172.16.0.3, user: ubuntu, password: Qcloud@123}
  roleGroups:
    etcd:
    - node1
    master: 
    - node1
    worker:
    - node1
    - node2
  controlPlaneEndpoint:
    ##Internal loadbalancer for apiservers 
    #internalLoadbalancer: haproxy

    domain: lb.kubesphere.local
    address: ""
    port: 6443
  kubernetes:
    version: {{ .Options.KubeVersion }}
    imageRepo: kubesphere
    clusterName: cluster.local
  network:
    plugin: calico
    kubePodsCIDR: 10.233.64.0/18
    kubeServiceCIDR: 10.233.0.0/18
  registry:
    registryMirrors: []
    insecureRegistries: []
  addons: []

{{ if .Options.KubeSphereEnabled }}
{{ .Options.KubeSphereConfigMap }}
{{ end }}
    `)))
)

// Options defineds the parameters of cluster configuration.
type Options struct {
	Name                string
	KubeVersion         string
	KubeSphereEnabled   bool
	KubeSphereConfigMap string
}

// GenerateClusterObjStr is used to generate cluster configuration content.
func GenerateClusterObjStr(opt *Options) (string, error) {
	return util.Render(ClusterObjTempl, util.Data{
		"KubeVersion": kubekeyapiv1alpha1.DefaultKubeVersion,
		"Options":     opt,
	})
}

// GenerateClusterObj is used to generate cluster configuration file
func GenerateClusterObj(k8sVersion, ksVersion, name, kubeconfig, clusterCfgPath string, ksEnabled, fromCluster bool) error {
	if fromCluster {
		err := GenerateConfigFromCluster(clusterCfgPath, kubeconfig, name)
		if err != nil {
			return err
		}
		return nil
	}

	opt := Options{}
	if name != "" {
		output := strings.Split(name, ".")
		opt.Name = output[0]
	} else {
		opt.Name = "sample"
	}
	if len(k8sVersion) == 0 {
		opt.KubeVersion = kubekeyapiv1alpha1.DefaultKubeVersion
	} else {
		opt.KubeVersion = k8sVersion
	}
	opt.KubeSphereEnabled = ksEnabled

	if ksEnabled {
		switch strings.TrimSpace(ksVersion) {
		case "v3.1.1", "latest":
			opt.KubeSphereConfigMap = kubesphere.V3_1_1
		case "v3.1.0":
			opt.KubeSphereConfigMap = kubesphere.V3_1_0
		case "v3.0.0":
			opt.KubeSphereConfigMap = kubesphere.V3_0_0
		case "v2.1.1":
			opt.KubeSphereConfigMap = kubesphere.V2_1_1
		default:
			return errors.New(fmt.Sprintf("Unsupported version: %s", strings.TrimSpace(ksVersion)))
		}
	}

	ClusterObjStr, err := GenerateClusterObjStr(&opt)
	if err != nil {
		return errors.Wrap(err, "Failed to generate cluster config")
	}
	ClusterObjStrBase64 := base64.StdEncoding.EncodeToString([]byte(ClusterObjStr))

	if clusterCfgPath != "" {
		CheckConfigFileStatus(clusterCfgPath)
		cmdStr := fmt.Sprintf("echo %s | base64 -d > %s", ClusterObjStrBase64, clusterCfgPath)
		output, err := exec.Command("/bin/sh", "-c", cmdStr).CombinedOutput()
		if err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to write config to %s: %s", clusterCfgPath, strings.TrimSpace(string(output))))
		}
	} else {
		currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return errors.Wrap(err, "Failed to get current dir")
		}
		CheckConfigFileStatus(fmt.Sprintf("%s/config-%s.yaml", currentDir, opt.Name))
		cmd := fmt.Sprintf("echo %s | base64 -d > %s/config-%s.yaml", ClusterObjStrBase64, currentDir, opt.Name)
		err1 := exec.Command("/bin/sh", "-c", cmd).Run()
		if err1 != nil {
			return err1
		}
	}

	return nil
}

// CheckConfigFileStatus is used to check the status of cluster configuration file.
func CheckConfigFileStatus(path string) {
	if util.IsExist(path) {
		reader := bufio.NewReader(os.Stdin)
	Loop:
		for {
			fmt.Printf("%s already exists. Are you sure you want to overwrite this config file? [yes/no]: ", path)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input != "" {
				switch input {
				case "yes":
					break Loop
				case "no":
					os.Exit(0)
				}
			}
		}
	}
}
