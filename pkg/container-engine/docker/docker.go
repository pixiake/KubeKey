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

package docker

import (
	"encoding/base64"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"text/template"
)

var (
	DockerConfigTempl = template.Must(template.New("DockerConfig").Parse(
		dedent.Dedent(`{
  "log-opts": {
    "max-size": "5m",
    "max-file":"3"
  },
  {{- if .Mirrors }}
  "registry-mirrors": [{{ .Mirrors }}],
  {{- end}}
  {{- if .InsecureRegistries }}
  "insecure-registries": [{{ .InsecureRegistries }}],
  {{- end}}
  "exec-opts": ["native.cgroupdriver=systemd"]
}
    `)))
)

func GenerateDockerConfig(mgr *manager.Manager) (string, error) {
	var Mirrors, InsecureRegistries string
	if mgr.Cluster.Registry.RegistryMirrors != nil {
		mirrors := []string{}
		for _, mirror := range mgr.Cluster.Registry.RegistryMirrors {
			mirrors = append(mirrors, fmt.Sprintf("\"%s\"", mirror))
		}
		Mirrors = strings.Join(mirrors, ", ")
	}
	if mgr.Cluster.Registry.InsecureRegistries != nil {
		registries := []string{}
		for _, repostry := range mgr.Cluster.Registry.InsecureRegistries {
			registries = append(registries, fmt.Sprintf("\"%s\"", repostry))
		}
		InsecureRegistries = strings.Join(registries, ", ")
	}
	return util.Render(DockerConfigTempl, util.Data{
		"Mirrors":            Mirrors,
		"InsecureRegistries": InsecureRegistries,
	})
}

func InstallerDocker(mgr *manager.Manager) error {
	if mgr.EtcdContainer {
		mgr.Logger.Infoln("Installing docker ...")

		if err := mgr.RunTaskOnAllNodes(installDockerOnNode, true); err != nil {
			return err
		}
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init nodes", mgr.Conditions[0].StartTime, metav1.Now(), true, 1); err != nil {
			return err
		}
	}

	return nil
}

func installDockerOnNode(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	dockerConfig, err := GenerateDockerConfig(mgr)
	if err != nil {
		return err
	}
	dockerConfigBase64 := base64.StdEncoding.EncodeToString([]byte(dockerConfig))
	output, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -z $(which docker) ] || [ ! -e /var/run/docker.sock ]; then curl https://kubernetes.pek3b.qingstor.com/tools/kubekey/docker-install.sh | sh && systemctl enable docker; if [ ! -f /etc/docker/daemon.json ]; then mkdir -p /etc/docker && echo %s | base64 -d > /etc/docker/daemon.json; fi; systemctl daemon-reload && systemctl restart docker; fi\"", dockerConfigBase64), 0, false)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to install docker:\n%s", output))
	}

	return nil
}
