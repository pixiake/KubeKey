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
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/kubesphere"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// ParseClusterCfg is used to generate Cluster object and cluster's name.
func ParseClusterCfg(clusterCfgPath, k8sVersion, ksVersion string, ksEnabled bool, logger *log.Logger) (*kubekeyapiv1alpha1.Cluster, string, error) {
	var (
		clusterCfg *kubekeyapiv1alpha1.Cluster
		objName    string
	)
	if len(clusterCfgPath) == 0 {
		currentUser, _ := user.Current()
		if currentUser.Username != "root" {
			return nil, "", errors.New(fmt.Sprintf("Current user is %s. Please use root!", currentUser.Username))
		}
		clusterCfg, objName = AllinoneCfg(currentUser, k8sVersion, ksVersion, ksEnabled, logger)
	} else {
		cfg, name, err := ParseCfg(clusterCfgPath, k8sVersion, ksVersion, ksEnabled)
		if err != nil {
			return nil, "", err
		}
		clusterCfg = cfg
		objName = name
	}
	return clusterCfg, objName, nil
}

// ParseCfg is used to parse the specified cluster configuration file.
func ParseCfg(clusterCfgPath, k8sVersion, ksVersion string, ksEnabled bool) (*kubekeyapiv1alpha1.Cluster, string, error) {
	var objName string
	clusterCfg := kubekeyapiv1alpha1.Cluster{}
	fp, err := filepath.Abs(clusterCfgPath)
	if err != nil {
		return nil, "", errors.Wrap(err, "Failed to look up current directory")
	}
	if len(k8sVersion) != 0 {
		_ = exec.Command("/bin/sh", "-c", fmt.Sprintf("sed -i \"/version/s/\\:.*/\\: %s/g\" %s", k8sVersion, fp)).Run()
	}
	file, err := os.Open(fp)
	if err != nil {
		return nil, "", errors.Wrap(err, "Unable to open the given cluster configuration file")
	}
	defer file.Close()
	b1 := bufio.NewReader(file)
	for {
		result := make(map[string]interface{})
		content, err := k8syaml.NewYAMLReader(b1).Read()
		if len(content) == 0 {
			break
		}
		if err != nil {
			return nil, "", errors.Wrap(err, "Unable to read the given cluster configuration file")
		}
		err = yaml.Unmarshal(content, &result)
		if err != nil {
			return nil, "", errors.Wrap(err, "Unable to unmarshal the given cluster configuration file")
		}
		if result["kind"] == "Cluster" {
			if err := yaml.Unmarshal(content, &clusterCfg); err != nil {
				return nil, "", errors.Wrap(err, "Unable to convert file to yaml")
			}
			metadata := result["metadata"].(map[interface{}]interface{})
			objName = metadata["name"].(string)
		}

		if result["kind"] == "ConfigMap" || result["kind"] == "ClusterConfiguration" {
			metadata := result["metadata"].(map[interface{}]interface{})
			labels := metadata["labels"].(map[interface{}]interface{})
			clusterCfg.Spec.KubeSphere.Enabled = true
			_, ok := labels["version"]
			if ok {
				switch labels["version"] {
				case "v3.1.1":
					clusterCfg.Spec.KubeSphere.Configurations = "---\n" + string(content)
					clusterCfg.Spec.KubeSphere.Version = "v3.1.1"
				case "v3.1.0":
					clusterCfg.Spec.KubeSphere.Configurations = "---\n" + string(content)
					clusterCfg.Spec.KubeSphere.Version = "v3.1.0"
				case "v3.0.0":
					clusterCfg.Spec.KubeSphere.Configurations = "---\n" + string(content)
					clusterCfg.Spec.KubeSphere.Version = "v3.0.0"
				case "v2.1.1":
					clusterCfg.Spec.KubeSphere.Configurations = "---\n" + string(content)
					clusterCfg.Spec.KubeSphere.Version = "v2.1.1"
				default:
					return nil, "", errors.Wrap(err, fmt.Sprintf("Unsupported version: %s", labels["version"]))
				}
			}
		}
	}

	if ksEnabled {
		clusterCfg.Spec.KubeSphere.Enabled = true
		switch strings.TrimSpace(ksVersion) {
		case "v3.1.1", "":
			clusterCfg.Spec.KubeSphere.Version = "v3.1.1"
			clusterCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_1
		case "v3.1.0":
			clusterCfg.Spec.KubeSphere.Version = "v3.1.0"
			clusterCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_0
		case "v3.0.0":
			clusterCfg.Spec.KubeSphere.Version = "v3.0.0"
			clusterCfg.Spec.KubeSphere.Configurations = kubesphere.V3_0_0
		case "v2.1.1":
			clusterCfg.Spec.KubeSphere.Version = "v2.1.1"
			clusterCfg.Spec.KubeSphere.Configurations = kubesphere.V2_1_1
		default:
			// make it be convenient to have a nightly build of KubeSphere
			if strings.HasPrefix(ksVersion, "nightly-") || ksVersion == "latest" {
				// this is not the perfect solution here, but it's not necessary to track down the exact version between the
				// nightly build and a released. So please keep update it with the latest release here.
				clusterCfg.Spec.KubeSphere.Version = ksVersion
				clusterCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_0
			} else {
				return nil, "", errors.New(fmt.Sprintf("Unsupported version: %s", strings.TrimSpace(ksVersion)))
			}
		}
	}

	return &clusterCfg, objName, nil
}

// AllinoneCfg is used to generate cluster object for all-in-one mode.
func AllinoneCfg(user *user.User, k8sVersion, ksVersion string, ksEnabled bool, logger *log.Logger) (*kubekeyapiv1alpha1.Cluster, string) {
	allinoneCfg := kubekeyapiv1alpha1.Cluster{}
	if output, err := exec.Command("/bin/sh", "-c", "if [ ! -f \"$HOME/.ssh/id_rsa\" ]; then ssh-keygen -t rsa -P \"\" -f $HOME/.ssh/id_rsa && ls $HOME/.ssh;fi;").CombinedOutput(); err != nil {
		log.Fatalf("Failed to generate public key: %v\n%s", err, string(output))
	}
	if output, err := exec.Command("/bin/sh", "-c", "echo \"\n$(cat $HOME/.ssh/id_rsa.pub)\" >> $HOME/.ssh/authorized_keys && awk ' !x[$0]++{print > \"'$HOME'/.ssh/authorized_keys\"}' $HOME/.ssh/authorized_keys").CombinedOutput(); err != nil {
		log.Fatalf("Failed to copy public key to authorized_keys: %v\n%s", err, string(output))
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Failed to get hostname: %v\n", err)
	}

	allinoneCfg.Spec.Hosts = append(allinoneCfg.Spec.Hosts, kubekeyapiv1alpha1.HostCfg{
		Name:            hostname,
		Address:         util.LocalIP(),
		InternalAddress: util.LocalIP(),
		Port:            kubekeyapiv1alpha1.DefaultSSHPort,
		User:            user.Name,
		Password:        "",
		PrivateKeyPath:  fmt.Sprintf("%s/.ssh/id_rsa", user.HomeDir),
		Arch:            runtime.GOARCH,
	})

	allinoneCfg.Spec.RoleGroups = kubekeyapiv1alpha1.RoleGroups{
		Etcd:   []string{hostname},
		Master: []string{hostname},
		Worker: []string{hostname},
	}
	if k8sVersion != "" {
		s := strings.Split(k8sVersion, "-")
		if len(s) > 1 {
			allinoneCfg.Spec.Kubernetes = kubekeyapiv1alpha1.Kubernetes{
				Version: s[0],
				Type:    s[1],
			}
		} else {
			allinoneCfg.Spec.Kubernetes = kubekeyapiv1alpha1.Kubernetes{
				Version: k8sVersion,
			}
		}
	} else {
		allinoneCfg.Spec.Kubernetes = kubekeyapiv1alpha1.Kubernetes{
			Version: kubekeyapiv1alpha1.DefaultKubeVersion,
		}
	}

	if ksEnabled {
		allinoneCfg.Spec.KubeSphere.Enabled = true
		ksVersion = strings.TrimSpace(ksVersion)
		switch ksVersion {
		case "v3.1.1", "":
			allinoneCfg.Spec.KubeSphere.Version = "v3.1.1"
			allinoneCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_1
		case "v3.1.0":
			allinoneCfg.Spec.KubeSphere.Version = "v3.1.0"
			allinoneCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_0
		case "v3.0.0":
			allinoneCfg.Spec.KubeSphere.Version = "v3.0.0"
			allinoneCfg.Spec.KubeSphere.Configurations = kubesphere.V3_0_0
		case "v2.1.1":
			allinoneCfg.Spec.KubeSphere.Version = "v2.1.1"
			allinoneCfg.Spec.KubeSphere.Configurations = kubesphere.V2_1_1
		default:
			// make it be convenient to have a nightly build of KubeSphere
			if strings.HasPrefix(ksVersion, "nightly-") || ksVersion == "latest" {
				// this is not the perfect solution here, but it's not necessary to track down the exact version between the
				// nightly build and a released. So please keep update it with the latest release here.
				allinoneCfg.Spec.KubeSphere.Version = ksVersion
				allinoneCfg.Spec.KubeSphere.Configurations = kubesphere.V3_1_0
			} else {
				logger.Fatalf("Unsupported version: %s", strings.TrimSpace(ksVersion))
			}
		}
	}

	return &allinoneCfg, hostname
}
