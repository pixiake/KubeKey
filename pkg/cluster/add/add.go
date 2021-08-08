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

package add

import (
	"fmt"
	"github.com/kubesphere/kubekey/pkg/cluster/install"
	"os"
	"path/filepath"

	kubekeyclientset "github.com/kubesphere/kubekey/clients/clientset/versioned"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/kubesphere/kubekey/pkg/container-engine/docker"
	"github.com/kubesphere/kubekey/pkg/etcd"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/executor"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func AddNodes(clusterCfgFile, k8sVersion, ksVersion string, logger *log.Logger, ksEnabled, verbose, skipCheck, skipPullImages, inCluster bool, downloadCmd string) error {
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return errors.Wrap(err, "Failed to get current dir")
	}
	if err := util.CreateDir(fmt.Sprintf("%s/kubekey", currentDir)); err != nil {
		return errors.Wrap(err, "Failed to create work dir")
	}

	cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, k8sVersion, ksVersion, ksEnabled, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to download cluster config")
	}

	var clientset *kubekeyclientset.Clientset
	if inCluster {
		c, err := kubekeycontroller.NewKubekeyClient()
		if err != nil {
			return err
		}
		clientset = c
	}

	//return Execute(executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, skipCheck, skipPullImages, false, inCluster, clientset))
	executorInstance := executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, skipCheck, skipPullImages, false, inCluster, clientset)
	executorInstance.DownloadCommand = func(path, url string) string {
		// this is an extension point for downloading tools, for example users can set the timeout, proxy or retry under
		// some poor network environment. Or users even can choose another cli, it might be wget.
		// perhaps we should have a build-in download function instead of totally rely on the external one
		return fmt.Sprintf(downloadCmd, path, url)
	}
	return Execute(executorInstance)
}

func ExecTasks(mgr *manager.Manager) error {

	skipCondition1 := mgr.Cluster.Kubernetes.Type == "k3s"

	addNodeTasks := []manager.Task{
		{Task: install.Precheck, ErrMsg: "Failed to precheck", Skip: skipCondition1},
		{Task: install.DownloadBinaries, ErrMsg: "Failed to download kube binaries"},
		{Task: install.InitOS, ErrMsg: "Failed to init OS"},
		{Task: docker.InstallerDocker, ErrMsg: "Failed to install docker", Skip: skipCondition1},
		{Task: install.PrePullImages, ErrMsg: "Failed to pre-pull images", Skip: skipCondition1},
		{Task: etcd.GetEtcdStatus, ErrMsg: "Failed to get etcd status"},
		{Task: etcd.GenerateEtcdCerts, ErrMsg: "Failed to generate etcd certs"},
		{Task: etcd.SyncEtcdCertsToMaster, ErrMsg: "Failed to sync etcd certs"},
		{Task: etcd.GenerateEtcdService, ErrMsg: "Failed to create etcd service"},
		{Task: etcd.SetupEtcdCluster, ErrMsg: "Failed to start etcd cluster"},
		{Task: etcd.RefreshEtcdConfig, ErrMsg: "Failed to refresh etcd configuration"},
		{Task: etcd.BackupEtcd, ErrMsg: "Failed to backup etcd data"},
		{Task: install.GetClusterStatus, ErrMsg: "Failed to get cluster status"},
		{Task: install.InstallKubeBinaries, ErrMsg: "Failed to install kube binaries"},
		{Task: install.JoinNodesToCluster, ErrMsg: "Failed to join node"},
		{Task: install.InstallInternalLoadbalancer, ErrMsg: "Failed to install internal load balancer", Skip: !mgr.Cluster.ControlPlaneEndpoint.IsInternalLBEnabled()},
	}

	for _, step := range addNodeTasks {
		if !step.Skip {
			if err := step.Run(mgr); err != nil {
				if mgr.InCluster {
					if err := kubekeycontroller.PatchNodeImportStatus(mgr, kubekeycontroller.Failed); err != nil {
						return err
					}
				}
				return errors.Wrap(err, step.ErrMsg)
			}
		}
	}

	if mgr.InCluster {
		if err := kubekeycontroller.PatchNodeImportStatus(mgr, kubekeycontroller.Success); err != nil {
			return err
		}

		if err := kubekeycontroller.UpdateStatus(mgr); err != nil {
			return err
		}
	}

	mgr.Logger.Infoln("Congratulations! Scaling cluster is successful.")

	return nil
}

func Execute(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}

	if mgr.InCluster {
		if err := kubekeycontroller.CreateNodeForCluster(mgr); err != nil {
			return err
		}
	}
	return ExecTasks(mgr)
}
