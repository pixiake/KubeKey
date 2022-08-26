/*
 Copyright 2021 The KubeSphere Authors.

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

package container

import (
	"fmt"
	"path/filepath"

	"github.com/kubesphere/kubekey/pkg/common"
	"github.com/kubesphere/kubekey/pkg/container/templates"
	"github.com/kubesphere/kubekey/pkg/core/connector"
	"github.com/kubesphere/kubekey/pkg/files"
	"github.com/kubesphere/kubekey/pkg/utils"
	"github.com/pkg/errors"
)

type SyncContainerd struct {
	common.KubeAction
}

func (s *SyncContainerd) Execute(runtime connector.Runtime) error {
	if err := utils.ResetTmpDir(runtime); err != nil {
		return err
	}

	binariesMapObj, ok := s.PipelineCache.Get(common.KubeBinaries + "-" + runtime.RemoteHost().GetArch())
	if !ok {
		return errors.New("get KubeBinary by pipeline cache failed")
	}
	binariesMap := binariesMapObj.(map[string]*files.KubeBinary)

	containerd, ok := binariesMap[common.Conatinerd]
	if !ok {
		return errors.New("get KubeBinary key containerd by pipeline cache failed")
	}

	dst := filepath.Join(common.TmpDir, containerd.FileName)
	if err := runtime.GetRunner().Scp(containerd.Path(), dst); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("sync containerd binaries failed"))
	}

	if _, err := runtime.GetRunner().SudoCmd(
		fmt.Sprintf("mkdir -p /usr/bin && tar -zxf %s && mv bin/* /usr/bin && rm -rf bin", dst),
		false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("install containerd binaries failed"))
	}
	return nil
}

type SyncCrictlBinaries struct {
	common.KubeAction
}

func (s *SyncCrictlBinaries) Execute(runtime connector.Runtime) error {
	if err := utils.ResetTmpDir(runtime); err != nil {
		return err
	}

	binariesMapObj, ok := s.PipelineCache.Get(common.KubeBinaries + "-" + runtime.RemoteHost().GetArch())
	if !ok {
		return errors.New("get KubeBinary by pipeline cache failed")
	}
	binariesMap := binariesMapObj.(map[string]*files.KubeBinary)

	crictl, ok := binariesMap[common.Crictl]
	if !ok {
		return errors.New("get KubeBinary key crictl by pipeline cache failed")
	}

	dst := filepath.Join(common.TmpDir, crictl.FileName)

	if err := runtime.GetRunner().Scp(crictl.Path(), dst); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("sync crictl binaries failed"))
	}

	if _, err := runtime.GetRunner().SudoCmd(
		fmt.Sprintf("mkdir -p /usr/bin && tar -zxf %s -C /usr/bin ", dst),
		false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("install crictl binaries failed"))
	}
	return nil
}

type EnableContainerd struct {
	common.KubeAction
}

func (e *EnableContainerd) Execute(runtime connector.Runtime) error {
	if _, err := runtime.GetRunner().SudoCmd(
		"systemctl daemon-reload && systemctl enable containerd && systemctl start containerd",
		false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("enable and start containerd failed"))
	}

	// install runc
	if err := utils.ResetTmpDir(runtime); err != nil {
		return err
	}

	binariesMapObj, ok := e.PipelineCache.Get(common.KubeBinaries + "-" + runtime.RemoteHost().GetArch())
	if !ok {
		return errors.New("get KubeBinary by pipeline cache failed")
	}
	binariesMap := binariesMapObj.(map[string]*files.KubeBinary)

	containerd, ok := binariesMap[common.Runc]
	if !ok {
		return errors.New("get KubeBinary key runc by pipeline cache failed")
	}

	dst := filepath.Join(common.TmpDir, containerd.FileName)
	if err := runtime.GetRunner().Scp(containerd.Path(), dst); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("sync runc binaries failed"))
	}

	if _, err := runtime.GetRunner().SudoCmd(
		fmt.Sprintf("install -m 755 %s /usr/local/sbin/runc", dst),
		false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("install runc binaries failed"))
	}
	return nil
}

type DisableContainerd struct {
	common.KubeAction
}

func (d *DisableContainerd) Execute(runtime connector.Runtime) error {
	if _, err := runtime.GetRunner().SudoCmd(
		"systemctl disable containerd && systemctl stop containerd", true); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("disable and stop containerd failed"))
	}

	// remove containerd related files
	files := []string{
		"/usr/local/sbin/runc",
		"/usr/bin/crictl",
		"/usr/bin/containerd*",
		"/usr/bin/ctr",
		filepath.Join("/etc/systemd/system", templates.ContainerdService.Name()),
		filepath.Join("/etc/containerd", templates.ContainerdConfig.Name()),
		filepath.Join("/etc", templates.CrictlConfig.Name()),
	}
	if d.KubeConf.Cluster.Registry.DataRoot != "" {
		files = append(files, d.KubeConf.Cluster.Registry.DataRoot)
	} else {
		files = append(files, "/var/lib/containerd")
	}

	for _, file := range files {
		_, _ = runtime.GetRunner().SudoCmd(fmt.Sprintf("rm -rf %s", file), true)
	}
	return nil
}
