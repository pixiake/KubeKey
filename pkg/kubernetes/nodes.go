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

package kubernetes

import (
	"encoding/base64"
	"fmt"
	"github.com/kubesphere/kubekey/pkg/kubernetes/config"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
)

// InstallKubeBinaries is used to install kubernetes' binaries to os' PATH.
func InstallKubeBinaries(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if !ExistNode(node) {
		if err := SyncKubeBinaries(mgr, node); err != nil {
			return err
		}

		if err := SetKubelet(mgr, node); err != nil {
			return err
		}
	}
	return nil
}

// ExistNode is used determine if the node already exists.
func ExistNode(node *kubekeyapiv1alpha1.HostCfg) bool {
	var version bool
	_, name := allNodesInfo[node.Name]
	if name && allNodesInfo[node.Name] != "" {
		version = true
	}
	_, ip := allNodesInfo[node.InternalAddress]
	return version || ip
}

// SyncKubeBinaries is used to sync kubernetes' binaries to each node.
func SyncKubeBinaries(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {

	tmpDir := "/tmp/kubekey"
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -d %s ]; then rm -rf %s ;fi\" && mkdir -p %s", tmpDir, tmpDir, tmpDir), 1, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create tmp dir")
	}

	currentDir, err1 := filepath.Abs(filepath.Dir(os.Args[0]))
	if err1 != nil {
		return errors.Wrap(err1, "Failed to get current dir")
	}

	filesDir := fmt.Sprintf("%s/%s/%s/%s", currentDir, kubekeyapiv1alpha1.DefaultPreDir, mgr.Cluster.Kubernetes.Version, node.Arch)

	kubeadm := "kubeadm"
	kubelet := "kubelet"
	kubectl := "kubectl"
	helm := "helm"
	kubecni := fmt.Sprintf("cni-plugins-linux-%s-%s.tgz", node.Arch, kubekeyapiv1alpha1.DefaultCniVersion)
	binaryList := []string{kubeadm, kubelet, kubectl, helm, kubecni}

	var cmdlist []string

	for _, binary := range binaryList {
		if err := mgr.Runner.ScpFile(fmt.Sprintf("%s/%s", filesDir, binary), fmt.Sprintf("%s/%s", "/tmp/kubekey", binary)); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to sync binaries"))
		}

		if strings.Contains(binary, "cni-plugins-linux") {
			cmdlist = append(cmdlist, fmt.Sprintf("mkdir -p /opt/cni/bin && tar -zxf %s/%s -C /opt/cni/bin", "/tmp/kubekey", binary))
		} else if strings.Contains(binary, "kubelet") {
			continue
		} else {
			cmdlist = append(cmdlist, fmt.Sprintf("cp -f /tmp/kubekey/%s /usr/local/bin/%s && chmod +x /usr/local/bin/%s", binary, binary, binary))
		}
	}
	cmd := strings.Join(cmdlist, " && ")
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", cmd), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to create kubelet link"))
	}

	return nil
}

// SetKubelet is used to configure the kubelet's startup parameters.
func SetKubelet(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", "cp -f /tmp/kubekey/kubelet /usr/local/bin/kubelet && chmod +x /usr/local/bin/kubelet"), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to create kubelet link"))
	}

	kubeletService, err1 := config.GenerateKubeletService()
	if err1 != nil {
		return err1
	}
	kubeletServiceBase64 := base64.StdEncoding.EncodeToString([]byte(kubeletService))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/systemd/system/kubelet.service\"", kubeletServiceBase64), 5, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubelet service")
	}

	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl disable kubelet && systemctl enable kubelet && ln -snf /usr/local/bin/kubelet /usr/bin/kubelet\"", 5, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to enable kubelet service")
	}

	kubeletEnv, err3 := config.GenerateKubeletEnv(node)
	if err3 != nil {
		return err3
	}
	kubeletEnvBase64 := base64.StdEncoding.EncodeToString([]byte(kubeletEnv))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p /etc/systemd/system/kubelet.service.d && echo %s | base64 -d > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf\"", kubeletEnvBase64), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubelet env")
	}

	return nil
}

const LocalServer = "server: https://127.0.0.1"

// UpdateKubeletConfig Update server filed in kubelet.conf
// When create a HA cluster by internal LB, we will set the server filed to 127.0.0.1:6443 (default) which in kubelet.conf.
// Because of that, the control plone node's kubelet connect the local api-server.
// And the work node's kubelet connect 127.0.0.1:6443 (default) that is proxy by the node's local nginx.
func UpdateKubeletConfig(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	output, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f /etc/kubernetes/kubelet.conf ] && echo 'kubelet.conf is exists.' || echo 'kubelet.conf is not exists.'\"", 0, true)
	if strings.Contains(output, "kubelet.conf is exists.") {
		// If the value is 'server: "https://127.0.0.1:6443"', return the function to avoid restart the kubelet.
		if out, err := mgr.Runner.ExecuteCmd("sudo sed -n '/server:.*/p' /etc/kubernetes/kubelet.conf", 1, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to get /etc/kubernetes/kubelet.conf")
		} else {
			if strings.Contains(strings.TrimSpace(out), LocalServer) {
				return nil
			}
		}

		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo sed -i 's#server:.*#server: https://127.0.0.1:%s#g' /etc/kubernetes/kubelet.conf", strconv.Itoa(mgr.Cluster.ControlPlaneEndpoint.Port)), 0, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to update /etc/kubernetes/kubelet.conf")
		}
	} else {
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to find /etc/kubernetes/kubelet.conf")
		}
		return errors.New("Failed to find /etc/kubernetes/kubelet.conf")
	}
	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && systemctl restart kubelet\"", 3, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to restart kubelet after update kubelet.conf")
	}
	return nil
}

// UpdateKubeproxyConfig is used to update kube-proxy configmap and restart tge kube-proxy pod.
func UpdateKubeproxyConfig(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		if out, err := mgr.Runner.ExecuteCmd("sudo -E /bin/bash -c \"set -o pipefail && /usr/local/bin/kubectl --kubeconfig /etc/kubernetes/admin.conf get configmap kube-proxy -n kube-system -o yaml "+
			"| sed -n '/server:.*/p' \"", 1, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to get kube-proxy config")
		} else {
			if strings.Contains(strings.TrimSpace(out), LocalServer) {
				return nil
			}
		}

		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"set -o pipefail && /usr/local/bin/kubectl --kubeconfig /etc/kubernetes/admin.conf get configmap kube-proxy -n kube-system -o yaml "+
			"| sed 's#server:.*#server: https://127.0.0.1:%s#g' "+
			"| /usr/local/bin/kubectl --kubeconfig /etc/kubernetes/admin.conf replace -f -\"", strconv.Itoa(mgr.Cluster.ControlPlaneEndpoint.Port)), 3, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to update kube-proxy config")
		}

		// Restart all kube-proxy pods to ensure that they load the new configmap.
		if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl --kubeconfig /etc/kubernetes/admin.conf delete pod -n kube-system -l k8s-app=kube-proxy --force --grace-period=0\"", 3, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to restart kube-proxy pod")
		}
	}
	return nil
}

// UpdateKubectlConfig is used to update the kubectl config. Make the value of field 'server' to set as 127.0.0.1:6443 (default).
// And all of the 'admin.conf' will connect to 127.0.0.1:6443 (default)
func UpdateKubectlConfig(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if node.IsMaster {
		output, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f /etc/kubernetes/admin.conf ] && echo 'admin.conf is exists.' || echo 'admin.conf is not exists.'\"", 0, false)
		if strings.Contains(output, "admin.conf is exists.") {
			if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo sed -i 's#server:.*#server: https://%s:%s#g' /etc/kubernetes/admin.conf", node.InternalAddress, strconv.Itoa(mgr.Cluster.ControlPlaneEndpoint.Port)), 0, false); err != nil {
				return errors.Wrap(errors.WithStack(err), "Failed to update /etc/kubernetes/kubelet.conf")
			}
		} else {
			if err != nil {
				return errors.Wrap(errors.WithStack(err), "Failed to find /etc/kubernetes/admin.conf")
			}
			return errors.New("Failed to find /etc/kubernetes/admin.conf")
		}
	}

	output2, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f ~/.kube/config ] && echo 'kubectl config is exists.' || echo 'kubectl config is not exists.'\"", 0, false)
	if strings.Contains(output2, "kubectl config is exists.") {
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo sed -i 's#server:.*#server: https://127.0.0.1:%s#g' ~/.kube/config", strconv.Itoa(mgr.Cluster.ControlPlaneEndpoint.Port)), 0, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to update ~/.kube/config")
		}
	} else {
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to find ~/.kube/config")
		}
		return errors.New("Failed to find ~/.kube/config")
	}
	return nil
}

// UpdateHostsFile is used to update the '/etc/hosts'. Make the 'lb.kubesphere.local' address to set as 127.0.0.1.
// All of the 'admin.conf' and '/.kube/config' will connect to 127.0.0.1:6443.
func UpdateHostsFile(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo sed -i 's#.* %s#127.0.0.1 %s#g' /etc/hosts", mgr.Cluster.ControlPlaneEndpoint.Domain, mgr.Cluster.ControlPlaneEndpoint.Domain), 0, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to update /etc/hosts")
	}
	return nil
}
