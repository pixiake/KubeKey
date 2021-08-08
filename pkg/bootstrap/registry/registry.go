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

package registry

import (
	"encoding/base64"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/executor"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

var registryCrt string

func InitRegistry(clusterCfgFile string, logger *log.Logger) error {
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return errors.Wrap(err, "Failed to get current dir")
	}
	if err := util.CreateDir(fmt.Sprintf("%s/kubekey", currentDir)); err != nil {
		return errors.Wrap(err, "Failed to create work dir")
	}

	cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to download cluster config")
	}

	return Execute(executor.NewExecutor(&cfg.Spec, objName, logger, "", true, true, true, false, false, nil))
}

func Execute(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}
	return ExecTasks(mgr)
}

func ExecTasks(mgr *manager.Manager) error {
	createTasks := []manager.Task{
		{Task: CreateRegistry, ErrMsg: "Failed to init operating system"},
	}

	for _, step := range createTasks {
		if err := step.Run(mgr); err != nil {
			return errors.Wrap(err, step.ErrMsg)
		}
	}

	mgr.Logger.Infoln("Init local registry successful.")

	return nil
}

func CreateRegistry(mgr *manager.Manager) error {
	user, _ := user.Current()
	if user.Username != "root" {
		return errors.New(fmt.Sprintf("Current user is %s. Please use root!", user.Username))
	}
	mgr.Logger.Infoln("Init local registry")

	if output, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("cp %s/registry /usr/local/bin/registry", mgr.WorkDir)).CombinedOutput(); err != nil {
		return errors.Wrapf(err, string(output))
	}

	if output, err := exec.Command("/bin/bash", "-c", "if [[ ! -f /etc/kubekey/registry/certs/domain.crt ]]; then "+
		"mkdir -p /etc/kubekey/registry/certs && "+
		"openssl req -newkey rsa:4096 -nodes -sha256 -keyout /etc/kubekey/registry/certs/domain.key -x509 -days 36500 -out /etc/kubekey/registry/certs/domain.crt -subj '/CN=dockerhub.kubekey.local';"+
		"fi").CombinedOutput(); err != nil {
		return errors.Wrapf(err, string(output))
	}

	registryCrtBase64Cmd := "cat /etc/kubekey/registry/certs/domain.crt | base64 --wrap=0"
	if output, err := exec.Command("/bin/sh", "-c", registryCrtBase64Cmd).CombinedOutput(); err != nil {
		return err
	} else {
		registryCrt = strings.TrimSpace(string(output))
	}

	registryConfig, err := GenerateRegistryConfig()
	if err != nil {
		return err
	}

	if output, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("echo %s | base64 -d > /etc/kubekey/registry/config.yaml", base64.StdEncoding.EncodeToString([]byte(registryConfig)))).CombinedOutput(); err != nil {
		return errors.Wrapf(err, string(output))
	}

	registryService, err := GenerateRegistryService()
	if err != nil {
		return err
	}

	if output, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("echo %s | base64 -d > /etc/systemd/system/registry.service", base64.StdEncoding.EncodeToString([]byte(registryService)))).CombinedOutput(); err != nil {
		return errors.Wrapf(err, string(output))
	}

	if output, err := exec.Command("/bin/sh", "-c", "systemctl daemon-reload && systemctl enable --now registry").CombinedOutput(); err != nil {
		return errors.Wrapf(err, string(output))
	}

	if err := mgr.RunTaskOnAllNodes(syncRegistryConfig, true); err != nil {
		return err
	}

	fmt.Print("\nLocal image registry created successfully. Address: dockerhub.kubekey.local:5000\n")
	return nil
}

func syncRegistryConfig(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo '%s  dockerhub.kubekey.local' >> /etc/hosts\"", util.LocalIP())+" && "+
		"sudo awk ' !x[$0]++{print > \"/etc/hosts\"}' /etc/hosts", 2, false); err != nil {
		return err
	}

	crtPaths := []string{"/etc/docker/certs.d/dockerhub.kubekey.local:5000", "/etc/kubekey/registry/certs"}
	for _, crtPath := range crtPaths {
		syncCrtCmd := fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s && echo %s | base64 -d > %s/ca.crt\"", crtPath, registryCrt, crtPath)
		if _, err := mgr.Runner.ExecuteCmd(syncCrtCmd, 1, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to sync registry crt")
		}
	}

	k3sRegistryConfig, err := GenerateK3sRegistryConfig()
	if err != nil {
		return err
	}
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p /etc/rancher/k3s && echo %s | base64 -d > /etc/rancher/k3s/registries.yaml\"", base64.StdEncoding.EncodeToString([]byte(k3sRegistryConfig))), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate k3s registries config")
	}

	return nil
}
