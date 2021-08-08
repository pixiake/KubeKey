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

package install

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	kubekeyclientset "github.com/kubesphere/kubekey/clients/clientset/versioned"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/addons"
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/kubesphere/kubekey/pkg/container-engine/docker"
	"github.com/kubesphere/kubekey/pkg/etcd"
	"github.com/kubesphere/kubekey/pkg/kubesphere"
	"github.com/kubesphere/kubekey/pkg/plugins/network"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/executor"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// CreateCluster is used to create cluster based on the given parameters or configuration file.
func CreateCluster(clusterCfgFile, k8sVersion, ksVersion string, logger *log.Logger, ksEnabled, verbose, skipCheck, skipPullImages, inCluster, deployLocalStorage bool, downloadCmd string) error {
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
	for _, host := range cfg.Spec.Hosts {
		if host.Name != strings.ToLower(host.Name) {
			return errors.New("Please do not use uppercase letters in hostname: " + host.Name)
		}
	}

	var clientset *kubekeyclientset.Clientset
	if inCluster {
		c, err := kubekeycontroller.NewKubekeyClient()
		if err != nil {
			return err
		}
		clientset = c
	}
	executorInstance := executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, skipCheck, skipPullImages, false, inCluster, clientset)

	executorInstance.DeployLocalStorage = deployLocalStorage

	executorInstance.DownloadCommand = func(path, url string) string {
		// this is an extension point for downloading tools, for example users can set the timeout, proxy or retry under
		// some poor network environment. Or users even can choose another cli, it might be wget.
		// perhaps we should have a build-in download function instead of totally rely on the external one
		return fmt.Sprintf(downloadCmd, path, url)
	}
	return Execute(executorInstance)
}

// ExecTasks is used to schedule and execute installation tasks.
func ExecTasks(mgr *manager.Manager) error {
	noNetworkPlugin := mgr.Cluster.Network.Plugin == "" || mgr.Cluster.Network.Plugin == "none"
	isK3s := mgr.Cluster.Kubernetes.Type == "k3s"
	createTasks := []manager.Task{
		{Task: Precheck, ErrMsg: "Failed to precheck", Skip: isK3s},
		{Task: DownloadBinaries, ErrMsg: "Failed to download kube binaries"},
		{Task: InitOS, ErrMsg: "Failed to init OS"},
		{Task: docker.InstallerDocker, ErrMsg: "Failed to install docker", Skip: isK3s},
		{Task: PrePullImages, ErrMsg: "Failed to pre-pull images", Skip: isK3s},
		{Task: etcd.GetEtcdStatus, ErrMsg: "Failed to get etcd status"},
		{Task: etcd.GenerateEtcdCerts, ErrMsg: "Failed to generate etcd certs"},
		{Task: etcd.SyncEtcdCertsToMaster, ErrMsg: "Failed to sync etcd certs"},
		{Task: etcd.GenerateEtcdService, ErrMsg: "Failed to create etcd service"},
		{Task: etcd.SetupEtcdCluster, ErrMsg: "Failed to start etcd cluster"},
		{Task: etcd.RefreshEtcdConfig, ErrMsg: "Failed to refresh etcd configuration"},
		{Task: etcd.BackupEtcd, ErrMsg: "Failed to backup etcd data"},
		{Task: GetClusterStatus, ErrMsg: "Failed to get cluster status"},
		{Task: InstallKubeBinaries, ErrMsg: "Failed to install kube binaries"},
		{Task: InitKubernetesCluster, ErrMsg: "Failed to init kubernetes cluster"},
		{Task: JoinNodesToCluster, ErrMsg: "Failed to join node"},
		{Task: InstallInternalLoadbalancer, ErrMsg: "Failed to install internal load balancer", Skip: !mgr.Cluster.ControlPlaneEndpoint.IsInternalLBEnabled()},
		{Task: network.DeployNetworkPlugin, ErrMsg: "Failed to deploy network plugin"},
		{Task: addons.InstallAddons, ErrMsg: "Failed to deploy addons", Skip: noNetworkPlugin},
		{Task: kubesphere.DeployLocalVolume, ErrMsg: "Failed to deploy localVolume", Skip: noNetworkPlugin || (!mgr.DeployLocalStorage && !mgr.KsEnable)},
		{Task: kubesphere.DeployKubeSphere, ErrMsg: "Failed to deploy kubesphere", Skip: noNetworkPlugin},
	}

	for _, step := range createTasks {
		if !step.Skip {
			if err := step.Run(mgr); err != nil {
				return errors.Wrap(err, step.ErrMsg)
			}
		}
	}

	if mgr.KsEnable && !noNetworkPlugin {
		mgr.Logger.Infoln(`Installation is complete.

Please check the result using the command:

       kubectl logs -n kubesphere-system $(kubectl get pod -n kubesphere-system -l app=ks-install -o jsonpath='{.items[0].metadata.name}') -f

`)
	} else {
		if mgr.InCluster {
			if err := kubekeycontroller.UpdateStatus(mgr); err != nil {
				return err
			}
			kubeConfigPath := filepath.Join(mgr.WorkDir, fmt.Sprintf("config-%s", mgr.ObjName))
			if kubeconfig, err := ioutil.ReadFile(kubeConfigPath); err != nil {
				return err
			} else {
				mgr.Kubeconfig = base64.StdEncoding.EncodeToString(kubeconfig)
				if err := kubekeycontroller.UpdateKubeSphereCluster(mgr); err != nil {
					return err
				}
				if err := kubekeycontroller.SaveKubeConfig(mgr); err != nil {
					return err
				}
			}
		}
		mgr.Logger.Infoln("Congratulations! Installation is successful.")
	}

	return nil
}

// Execute executes the tasks based on the parameters in the Manager.
func Execute(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}

	return ExecTasks(mgr)
}
