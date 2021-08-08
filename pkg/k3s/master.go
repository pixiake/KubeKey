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

package k3s

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/k3s/config"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
)

var (
	clusterIsExist = false
	allNodesInfo   = map[string]string{}
	clusterStatus  = map[string]string{
		"version":       "",
		"joinMasterCmd": "",
		"joinWorkerCmd": "",
		"clusterInfo":   "",
		"nodeToken":     "",
	}
)

// GetClusterStatus is used to fetch status and info from cluster.
func GetClusterStatus(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		if clusterStatus["clusterInfo"] == "" {
			output, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f /etc/systemd/system/k3s.service ] && echo 'Cluster already exists.' || echo 'Cluster will be created.'\"", 0, true)
			if strings.Contains(output, "Cluster will be created") {
				clusterIsExist = false
			} else {
				if err != nil {
					return errors.Wrap(errors.WithStack(err), "Failed to find /etc/systemd/system/k3s.service")
				}
				clusterIsExist = true
				if output, err := mgr.Runner.ExecuteCmd("sudo k3s --version | grep 'k3s' | awk '{print $3}'", 0, true); err != nil {
					return errors.Wrap(errors.WithStack(err), "Failed to find current version")
				} else {
					clusterStatus["version"] = output
				}
				kubeCfgBase64Cmd := "cat /etc/rancher/k3s/k3s.yaml | base64 --wrap=0"
				kubeConfigStr, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", kubeCfgBase64Cmd), 1, false)
				if err1 != nil {
					return errors.Wrap(errors.WithStack(err1), "Failed to get cluster kubeconfig")
				}
				clusterStatus["kubeconfig"] = kubeConfigStr
				if err := loadKubeConfig(mgr); err != nil {
					return err
				}
				if err := getJoinNodesCmd(mgr); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// InitK3sCluster is used to init a new cluster.
func InitK3sCluster(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 && !clusterIsExist {
		if err := setK3sSystemdConfig(mgr, node, ""); err != nil {
			return err
		}
		kubeletEnv, err3 := config.GenerateK3sEnv(mgr, node, "")
		if err3 != nil {
			return err3
		}
		kubeletEnvBase64 := base64.StdEncoding.EncodeToString([]byte(kubeletEnv))
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/systemd/system/k3s.service.env\"", kubeletEnvBase64), 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to generate kubelet env")
		}

		_, err1 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && systemctl enable --now k3s\"", 1, false)
		if err1 != nil {
			return errors.Wrap(errors.WithStack(err1), "Failed to start k3s")
		}

		if err := GetKubeConfig(mgr); err != nil {
			return err
		}

		if !node.IsWorker {
			addTaintForMasterCmd := fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl taint nodes %s node-role.kubernetes.io/master=effect:NoSchedule --overwrite\"", node.Name)
			_, _ = mgr.Runner.ExecuteCmd(addTaintForMasterCmd, 5, true)
		}

		if err := addWorkerLabel(mgr, node); err != nil {
			return err
		}
		clusterIsExist = true

		kubeCfgBase64Cmd := "cat /etc/rancher/k3s/k3s.yaml | base64 --wrap=0"
		kubeConfigStr, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", kubeCfgBase64Cmd), 1, false)
		if err1 != nil {
			return errors.Wrap(errors.WithStack(err1), "Failed to get cluster kubeconfig")
		}
		clusterStatus["kubeconfig"] = kubeConfigStr

		if err := getJoinNodesCmd(mgr); err != nil {
			return err
		}
		if err := loadKubeConfig(mgr); err != nil {
			return err
		}
	}

	return nil
}

// GetKubeConfig is used to copy k3s.yaml to ~/.kube/config .
func GetKubeConfig(mgr *manager.Manager) error {
	createConfigDirCmd := "mkdir -p /root/.kube && mkdir -p $HOME/.kube"
	getKubeConfigCmd := "cp -f /etc/rancher/k3s/k3s.yaml /root/.kube/config"
	getKubeConfigCmdUsr := "cp -f /etc/rancher/k3s/k3s.yaml $HOME/.kube/config"
	chownKubeConfig := "chown $(id -u):$(id -g) $HOME/.kube/config"

	cmd := strings.Join([]string{createConfigDirCmd, getKubeConfigCmd, getKubeConfigCmdUsr, chownKubeConfig}, " && ")
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", cmd), 2, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to init kubernetes cluster")
	}
	return nil
}

func addWorkerLabel(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if node.IsWorker {
		addWorkerLabelCmd := fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl label --overwrite node %s node-role.kubernetes.io/worker=\"", node.Name)
		_, _ = mgr.Runner.ExecuteCmd(addWorkerLabelCmd, 5, true)
	}
	return nil
}

func getJoinNodesCmd(mgr *manager.Manager) error {
	if err := getJoinCmd(mgr); err != nil {
		return err
	}
	return nil
}

func getJoinCmd(mgr *manager.Manager) error {
	nodeTokenBase64Cmd := "cat /var/lib/rancher/k3s/server/node-token"
	nodeTokenStr, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", nodeTokenBase64Cmd), 1, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to get cluster node token")
	}
	clusterStatus["nodeToken"] = nodeTokenStr

	for i := 0; i < 6; i++ {
		output, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl --no-headers=true get nodes -o custom-columns=:metadata.name,:status.nodeInfo.kubeletVersion,:status.addresses\"", 5, true)
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to get cluster info")
		}
		if strings.TrimSpace(output) != "" {
			clusterStatus["clusterInfo"] = output
			break
		}
		time.Sleep(5 * time.Second)
	}
	ipv4Regexp, err4 := regexp.Compile("[\\d]+\\.[\\d]+\\.[\\d]+\\.[\\d]+")
	if err4 != nil {
		return err4
	}
	ipv6Regexp, err5 := regexp.Compile("[a-f0-9]{1,4}(:[a-f0-9]{1,4}){7}|[a-f0-9]{1,4}(:[a-f0-9]{1,4}){0,7}::[a-f0-9]{0,4}(:[a-f0-9]{1,4}){0,7}")
	if err5 != nil {
		return err5
	}
	tmp := strings.Split(clusterStatus["clusterInfo"], "\r\n")
	if len(tmp) >= 1 {
		for i := 0; i < len(tmp); i++ {
			if ipv4 := ipv4Regexp.FindStringSubmatch(tmp[i]); len(ipv4) != 0 {
				allNodesInfo[ipv4[0]] = ipv4[0]
			}
			if ipv6 := ipv6Regexp.FindStringSubmatch(tmp[i]); len(ipv6) != 0 {
				allNodesInfo[ipv6[0]] = ipv6[0]
			}
			if len(strings.Fields(tmp[i])) > 3 {
				allNodesInfo[strings.Fields(tmp[i])[0]] = strings.Fields(tmp[i])[1]
			} else {
				allNodesInfo[strings.Fields(tmp[i])[0]] = ""
			}
		}
	}
	kubeCfgBase64Cmd := "cat  /etc/rancher/k3s/k3s.yaml | base64 --wrap=0"
	output, err6 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", kubeCfgBase64Cmd), 1, false)
	if err6 != nil {
		return errors.Wrap(errors.WithStack(err6), "Failed to get cluster kubeconfig")
	}
	clusterStatus["kubeconfig"] = output
	return nil
}

// JoinNodesToCluster is used to join node to Cluster.
func JoinNodesToCluster(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if !ExistNode(node) {
		if node.IsMaster {
			err := addMaster(mgr, node)
			if err != nil {
				return err
			}
			err2 := addWorkerLabel(mgr, node)
			if err2 != nil {
				return err2
			}
			if !node.IsWorker {
				addTaintForMasterCmd := fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl taint nodes %s node-role.kubernetes.io/master=effect:NoSchedule --overwrite\"", node.Name)
				_, _ = mgr.Runner.ExecuteCmd(addTaintForMasterCmd, 5, true)
			}
		}
		if node.IsWorker && !node.IsMaster {
			err := addWorker(mgr, node)
			if err != nil {
				return err
			}
			err1 := addWorkerLabel(mgr, node)
			if err1 != nil {
				return err1
			}
		}
	}
	return nil
}

func addMaster(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if err := setK3sSystemdConfig(mgr, node, ""); err != nil {
		return err
	}
	kubeletEnv, err3 := config.GenerateK3sEnv(mgr, node, "")
	if err3 != nil {
		return err3
	}
	kubeletEnvBase64 := base64.StdEncoding.EncodeToString([]byte(kubeletEnv))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/systemd/system/k3s.service.env\"", kubeletEnvBase64), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubelet env")
	}

	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && systemctl enable --now k3s\"", 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to up k3s")
	}

	return nil
}

func addWorker(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if err := setK3sSystemdConfig(mgr, node, clusterStatus["nodeToken"]); err != nil {
		return err
	}
	kubeletEnv, err3 := config.GenerateK3sEnv(mgr, node, clusterStatus["nodeToken"])
	if err3 != nil {
		return err3
	}
	kubeletEnvBase64 := base64.StdEncoding.EncodeToString([]byte(kubeletEnv))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/systemd/system/k3s.service.env\"", kubeletEnvBase64), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubelet env")
	}

	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && systemctl enable --now k3s\"", 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to up k3s")
	}

	createConfigDirCmd := "mkdir -p /root/.kube && mkdir -p $HOME/.kube"
	chownKubeConfig := "chown $(id -u):$(id -g) -R $HOME/.kube"

	kubeconfigStr, err := base64.StdEncoding.DecodeString(clusterStatus["kubeconfig"])
	if err != nil {
		return err
	}

	oldServer := "server: https://127.0.0.1:6443"
	newServer := fmt.Sprintf("server: https://%s:%d", mgr.Cluster.ControlPlaneEndpoint.Domain, mgr.Cluster.ControlPlaneEndpoint.Port)
	newKubeconfigStr := strings.Replace(string(kubeconfigStr), oldServer, newServer, -1)

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", createConfigDirCmd), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create kube dir")
	}
	syncKubeconfigForRootCmd := fmt.Sprintf("sudo -E /bin/sh -c \"echo '%s' > %s\"", newKubeconfigStr, "/root/.kube/config")
	syncKubeconfigForUserCmd := fmt.Sprintf("echo '%s' > %s && %s", newKubeconfigStr, "$HOME/.kube/config", chownKubeConfig)

	if _, err := mgr.Runner.ExecuteCmd(syncKubeconfigForRootCmd, 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to sync kube config")
	}
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", syncKubeconfigForUserCmd), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to sync kube config")
	}

	return nil
}

func loadKubeConfig(mgr *manager.Manager) error {
	kubeConfigPath := filepath.Join(mgr.WorkDir, fmt.Sprintf("config-%s", mgr.ObjName))
	kubeconfigStr, err := base64.StdEncoding.DecodeString(clusterStatus["kubeconfig"])
	if err != nil {
		return err
	}

	oldServer := "server: https://127.0.0.1:6443"
	newServer := fmt.Sprintf("server: https://%s:%d", mgr.Cluster.ControlPlaneEndpoint.Address, mgr.Cluster.ControlPlaneEndpoint.Port)
	newKubeconfigStr := strings.Replace(string(kubeconfigStr), oldServer, newServer, -1)

	if err := ioutil.WriteFile(kubeConfigPath, []byte(newKubeconfigStr), 0644); err != nil {
		return err
	}

	return nil
}

func AddLabelsForNodes(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	for k, v := range node.Labels {
		addLabelCmd := fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl label --overwrite node %s %s=%s\"", node.Name, k, v)
		_, _ = mgr.Runner.ExecuteCmd(addLabelCmd, 5, true)
	}

	return nil
}
