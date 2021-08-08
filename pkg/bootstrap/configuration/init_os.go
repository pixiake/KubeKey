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

package configuration

import (
	"encoding/base64"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
)

const (
	binDir                       = "/usr/local/bin"
	kubeConfigDir                = "/etc/kubernetes"
	kubeCertDir                  = "/etc/kubernetes/pki"
	kubeManifestDir              = "/etc/kubernetes/manifests"
	kubeScriptDir                = "/usr/local/bin/kube-scripts"
	kubeletFlexvolumesPluginsDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec"
)

// InitOsOnNode is uesed to initialize the operating system. shuch as: override hostname, configuring kernel parameters, etc.
func InitOsOnNode(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {

	_ = addUsers(mgr, node)

	if err := createDirectories(mgr, node); err != nil {
		return err
	}

	tmpDir := "/tmp/kubekey"
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -d %s ]; then rm -rf %s ;fi\" && mkdir -p %s", tmpDir, tmpDir, tmpDir), 1, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create tmp dir")
	}

	_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"hostnamectl set-hostname %s && sed -i '/^127.0.1.1/s/.*/127.0.1.1      %s/g' /etc/hosts\"", node.Name, node.Name), 1, false)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to override hostname")
	}

	initOsScript, err2 := InitOsScript(mgr)
	if err2 != nil {
		return err2
	}

	str := base64.StdEncoding.EncodeToString([]byte(initOsScript))
	_, err3 := mgr.Runner.ExecuteCmd(fmt.Sprintf("echo %s | base64 -d > %s/initOS.sh && chmod +x %s/initOS.sh", str, tmpDir, tmpDir), 1, false)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err3), "Failed to generate init os script")
	}

	_, err4 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo cp %s/initOS.sh %s && sudo %s/initOS.sh", tmpDir, kubeScriptDir, kubeScriptDir), 1, true)
	if err4 != nil {
		return errors.Wrap(errors.WithStack(err4), "Failed to configure operating system")
	}
	return nil
}

func addUsers(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"useradd -M -c 'Kubernetes user' -s /sbin/nologin -r kube || :\"", 1, false); err != nil {
		return err
	}

	if node.IsEtcd {
		if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"useradd -M -c 'Etcd user' -s /sbin/nologin -r etcd || :\"", 1, false); err != nil {
			return err
		}
	}

	return nil
}

func createDirectories(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	dirs := []string{binDir, kubeConfigDir, kubeCertDir, kubeManifestDir, kubeScriptDir, kubeletFlexvolumesPluginsDir}
	for _, dir := range dirs {
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s\"", dir), 1, false); err != nil {
			return err
		}
		if dir == kubeletFlexvolumesPluginsDir {
			if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"chown kube -R %s\"", "/usr/libexec/kubernetes"), 1, false); err != nil {
				return err
			}
		} else {
			if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"chown kube -R %s\"", dir), 1, false); err != nil {
				return err
			}
		}
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && chown kube -R %s\"", "/etc/cni/net.d", "/etc/cni"), 1, false); err != nil {
		return err
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && chown kube -R %s\"", "/opt/cni/bin", "/opt/cni"), 1, false); err != nil {
		return err
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && chown kube -R %s\"", "/var/lib/calico", "/var/lib/calico"), 1, false); err != nil {
		return err
	}

	if node.IsEtcd {
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && chown etcd -R %s\"", "/var/lib/etcd", "/var/lib/etcd"), 1, false); err != nil {
			return err
		}
	}

	return nil
}
