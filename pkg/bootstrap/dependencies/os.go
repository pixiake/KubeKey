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

package dependencies

import (
	"encoding/base64"
	"fmt"
	osrelease "github.com/dominodatalab/os-release"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/container-engine/docker"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

var registryCrt string

func InitOS(mgr *manager.Manager) error {
	user, _ := user.Current()
	if user.Username != "root" {
		return errors.New(fmt.Sprintf("Current user is %s. Please use root!", user.Username))
	}
	mgr.Logger.Infoln("Init operating system")

	if err := mgr.RunTaskOnAllNodes(initOS, true); err != nil {
		return err
	}

	if mgr.AddImagesRepo {
		if output, err := exec.Command("/bin/bash", "-c", "if [[ ! \"$(docker ps --filter 'name=kubekey-registry' --format '{{.Names}}')\" =~ 'kubekey-registry' ]]; then "+
			"mkdir -p /opt/registry/certs && "+
			"openssl req -newkey rsa:4096 -nodes -sha256 -keyout /opt/registry/certs/domain.key -x509 -days 36500 -out /opt/registry/certs/domain.crt -subj '/CN=dockerhub.kubekey.local';"+
			"fi").CombinedOutput(); err != nil {
			return errors.Wrapf(err, string(output))
		}
		registryCrtBase64Cmd := "cat /opt/registry/certs/domain.crt | base64 --wrap=0"
		if output, err := exec.Command("/bin/sh", "-c", registryCrtBase64Cmd).CombinedOutput(); err != nil {
			return err
		} else {
			registryCrt = strings.TrimSpace(string(output))
		}
		if output, err := exec.Command("/bin/bash", "-c",
			"if [[ ! \"$(docker ps --filter 'name=kubekey-registry' --format '{{.Names}}')\" =~ 'kubekey-registry' ]]; then "+
				"docker run -d --restart=always --name kubekey-registry "+
				"-v /opt/registry/certs:/certs -v /mnt/registry:/var/lib/registry "+
				"-e REGISTRY_HTTP_ADDR=0.0.0.0:443 -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key "+
				"-p 443:443 registry:2; fi").CombinedOutput(); err != nil {
			return errors.Wrapf(err, string(output))
		}

		if err := mgr.RunTaskOnAllNodes(initImagesRepo, true); err != nil {
			return err
		}

		fmt.Print("\nLocal images repository created successfully. Address: dockerhub.kubekey.local\n")
	}
	return nil
}

func initOS(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	initFlag, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -z $(which docker) ] || [ ! -e /var/run/docker.sock ]; then echo needToInit; fi\""), 1, false)
	if err1 != nil {
		return err1
	}

	if strings.Contains(initFlag, "needToInit") {
		osReleaseStr, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"cat /etc/os-release\"", 2, false)
		if err != nil {
			return err
		}
		osrData := osrelease.Parse(strings.Replace(osReleaseStr, "\r\n", "\n", -1))

		pkgToolStr, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"if [ ! -z $(which yum 2>/dev/null) ]; then echo rpm; elif [ ! -z $(which apt 2>/dev/null) ]; then echo deb; fi\"", 2, false)
		if err != nil {
			return err
		}

		dockerConfig, err := docker.GenerateDockerConfig(mgr)
		if err != nil {
			return err
		}
		dockerConfigBase64 := base64.StdEncoding.EncodeToString([]byte(dockerConfig))
		mgr.Logger.Info(fmt.Sprintf("Start initializing %s [%s]\n", node.Name, node.InternalAddress))
		if mgr.SourcesDir == "" {
			switch strings.TrimSpace(pkgToolStr) {
			case "deb":
				if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \""+
					"apt update;"+
					"apt install socat conntrack ipset ebtables chrony -y;"+
					"\"", 2, false); err != nil {
					return err
				}
			case "rpm":
				if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \""+
					"yum install yum-utils openssl socat conntrack ipset ebtables chrony -y\"", 2, false); err != nil {
					return err
				}
			default:
				return errors.New(fmt.Sprintf("Unsupported operating system: %s", osrData.ID))
			}

			output, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -z $(which docker) ] || [ ! -e /var/run/docker.sock ]; then curl https://kubernetes.pek3b.qingstor.com/tools/kubekey/docker-install.sh | sh && systemctl enable docker; if [ ! -f /etc/docker/daemon.json ]; then mkdir -p /etc/docker && echo %s | base64 -d > /etc/docker/daemon.json; fi; systemctl daemon-reload && systemctl restart docker; fi\"", dockerConfigBase64), 0, false)
			if err1 != nil {
				return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to install docker:\n%s", output))
			}
		} else {
			fp, err := filepath.Abs(mgr.SourcesDir)
			if err != nil {
				return errors.Wrap(err, "Failed to look up current directory")
			}

			switch strings.TrimSpace(pkgToolStr) {
			case "deb":
				dirName := fmt.Sprintf("%s-%s-%s-debs", osrData.ID, osrData.VersionID, node.Arch)
				_ = mgr.Runner.ScpFile(fmt.Sprintf("%s/%s.tar.gz", fp, dirName), "/tmp")
				if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \""+
					fmt.Sprintf("tar -zxvf /tmp/%s.tar.gz -C /tmp && dpkg -iR --force-all /tmp/%s", dirName, dirName)+"\"", 2, false); err != nil {
					return err
				}
			case "rpm":
				dirName := fmt.Sprintf("%s-%s-%s-rpms", osrData.ID, osrData.VersionID, node.Arch)
				_ = mgr.Runner.ScpFile(fmt.Sprintf("%s/%s.tar.gz", fp, dirName), "/tmp")
				if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \""+
					fmt.Sprintf("tar -zxvf /tmp/%s.tar.gz -C /tmp && rpm -Uvh --force --nodeps /tmp/%s/*rpm", dirName, dirName)+"\"", 2, false); err != nil {
					return err
				}
			default:
				return errors.New(fmt.Sprintf("Unsupported operating system: %s", osrData.ID))
			}

			_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"systemctl start docker && systemctl enable docker; if [ ! -f /etc/docker/daemon.json ]; then mkdir -p /etc/docker && echo %s | base64 -d > /etc/docker/daemon.json; fi; systemctl daemon-reload && systemctl restart docker\"", dockerConfigBase64), 0, false)
		}
		mgr.Logger.Info(fmt.Sprintf("Complete initialization %s [%s]\n", node.Name, node.InternalAddress))
	}

	return nil
}

func initImagesRepo(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	crtPath := "/etc/docker/certs.d/dockerhub.kubekey.local"
	syncKubeconfigForRootCmd := fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && echo %s | base64 -d > %s/ca.crt\"", crtPath, registryCrt, crtPath)
	if _, err := mgr.Runner.ExecuteCmd(syncKubeconfigForRootCmd, 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to sync registry crt")
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo '%s  dockerhub.kubekey.local' >> /etc/hosts\"", util.LocalIP())+" && "+
		"sudo awk ' !x[$0]++{print > \"/etc/hosts\"}' /etc/hosts", 2, false); err != nil {
		return err
	}

	return nil
}
