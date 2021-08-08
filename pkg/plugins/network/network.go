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

package network

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/plugins/network/calico"
	"github.com/kubesphere/kubekey/pkg/plugins/network/cilium"
	"github.com/kubesphere/kubekey/pkg/plugins/network/flannel"
	"github.com/kubesphere/kubekey/pkg/plugins/network/kubeovn"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	versionutil "k8s.io/apimachinery/pkg/util/version"
)

// DeployNetworkPlugin is used to deploy network plugin.
func DeployNetworkPlugin(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Deploying network plugin ...")

	if err := mgr.RunTaskOnMasterNodes(deployNetworkPlugin, true); err != nil {
		return err
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init control plane", mgr.Conditions[3].StartTime, metav1.Now(), true, 4); err != nil {
			return err
		}
	}

	return nil
}

func deployNetworkPlugin(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		switch mgr.Cluster.Network.Plugin {
		case "calico":
			if err := deployCalico(mgr); err != nil {
				return err
			}
		case "flannel":
			if err := deployFlannel(mgr); err != nil {
				return err
			}
		case "cilium":
			if err := deployCilium(mgr); err != nil {
				return err
			}
		case "kubeovn":
			if err := deployKubeovn(mgr); err != nil {
				return err
			}
		case "", "none":
			mgr.Logger.Warningln("No network plugin specified, installation ends here !")
			return nil
		case "custom":
			return nil
		default:
			return errors.New(fmt.Sprintf("This network plugin is not supported: %s", mgr.Cluster.Network.Plugin))
		}
	}
	return nil
}

func deployCalico(mgr *manager.Manager) error {

	var calicoContent string
	cmp, err := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version).Compare("v1.16.0")
	if err != nil {
		return err
	}
	if cmp == -1 {
		calicoContentStr, err := calico.GenerateCalicoFilesOld(mgr)
		if err != nil {
			return err
		}
		calicoContent = calicoContentStr
	} else {
		calicoContentStr, err := calico.GenerateCalicoFilesNew(mgr)
		if err != nil {
			return err
		}
		calicoContent = calicoContentStr
	}

	err1 := ioutil.WriteFile(fmt.Sprintf("%s/network-plugin.yaml", mgr.WorkDir), []byte(calicoContent), 0644)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to generate network plugin manifests: %s/network-plugin.yaml", mgr.WorkDir))
	}

	calicoBase64, err1 := exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< network-plugin.yaml | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to read network plugin manifests")
	}

	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"", strings.TrimSpace(string(calicoBase64)), "/etc/kubernetes"), 2, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate network plugin manifests")
	}

	_, err3 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/network-plugin.yaml --force\"", 5, true)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err3), "Failed to deploy network plugin")
	}
	return nil
}

func deployFlannel(mgr *manager.Manager) error {
	flannelContent, err := flannel.GenerateFlannelFiles(mgr)
	if err != nil {
		return err
	}
	err1 := ioutil.WriteFile(fmt.Sprintf("%s/network-plugin.yaml", mgr.WorkDir), []byte(flannelContent), 0644)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to generate network plugin manifests: %s/network-plugin.yaml", mgr.WorkDir))
	}

	flannelBase64, err1 := exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< network-plugin.yaml | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to read network plugin manifests")
	}

	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"", strings.TrimSpace(string(flannelBase64)), "/etc/kubernetes"), 2, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate network plugin manifests")
	}

	_, err3 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/network-plugin.yaml --force\"", 5, true)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to deploy network plugin")
	}
	return nil
}

func deployCilium(mgr *manager.Manager) error {

	ciliumContent, err := cilium.GenerateCiliumFiles(mgr)
	if err != nil {
		return err
	}
	err1 := ioutil.WriteFile(fmt.Sprintf("%s/network-plugin.yaml", mgr.WorkDir), []byte(ciliumContent), 0644)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to generate network plugin manifests: %s/network-plugin.yaml", mgr.WorkDir))
	}

	ciliumBase64, err1 := exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< network-plugin.yaml | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to read network plugin manifests")
	}

	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"", strings.TrimSpace(string(ciliumBase64)), "/etc/kubernetes"), 2, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate network plugin manifests")
	}

	_, err3 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/network-plugin.yaml --force\"", 5, true)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to deploy network plugin")
	}
	return nil
}

func deployKubeovn(mgr *manager.Manager) error {
	if mgr.Runner.Index == 0 {
		if err := kubeovn.LabelNode(mgr); err != nil {
			return errors.Wrap(errors.WithStack(err), err.Error())
		}

		if mgr.Cluster.Network.Kubeovn.EnableSSL {
			if err := kubeovn.GenerateSSL(mgr); err != nil {
				return errors.Wrap(errors.WithStack(err), err.Error())
			}
		}
	}

	var kubeovnContent string
	var err error

	cmp, err := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version).Compare("v1.16.0")
	if err != nil {
		return err
	}
	if cmp == -1 {
		kubeovnContent, err = kubeovn.GenerateKubeovnFilesOld(mgr)
	} else {
		kubeovnContent, err = kubeovn.GenerateKubeovnFilesNew(mgr)
	}
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/network-plugin.yaml", mgr.WorkDir), []byte(kubeovnContent), 0644)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to generate network plugin manifests: %s/network-plugin.yaml", mgr.WorkDir))
	}

	kubeovnBase64, err1 := exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< network-plugin.yaml | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to read network plugin manifests")
	}

	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"", strings.TrimSpace(string(kubeovnBase64)), "/etc/kubernetes"), 2, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate network plugin manifests")
	}

	_, err3 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/network-plugin.yaml\"", 5, true)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err3), "Failed to deploy network plugin")
	}

	// deploy kubectl plugin kubectl-ko
	kubectlKo, err := kubeovn.GenerateKubectlKo(mgr)
	if err != nil {
		return err
	}

	str := base64.StdEncoding.EncodeToString([]byte(kubectlKo))
	_, err4 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /usr/local/bin/kubectl-ko && chmod +x /usr/local/bin/kubectl-ko\"", str), 1, true)
	if err4 != nil {
		return errors.Wrap(errors.WithStack(err4), "Failed to mv kubectl-ko to /usr/local/bin")
	}

	return nil
}
