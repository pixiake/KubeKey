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

package executor

import (
	"fmt"
	"github.com/kubesphere/kubekey/pkg/connector/ssh"
	"os"
	"path/filepath"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeyclientset "github.com/kubesphere/kubekey/clients/clientset/versioned"
	"github.com/kubesphere/kubekey/pkg/connector"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Executor struct {
	ObjName            string
	Cluster            *kubekeyapiv1alpha1.ClusterSpec
	Logger             *log.Logger
	SourcesDir         string
	Debug              bool
	SkipCheck          bool
	SkipPullImages     bool
	DeployLocalStorage bool
	AddImagesRepo      bool
	InCluster          bool
	ClientSet          *kubekeyclientset.Clientset
	DownloadCommand    func(path, url string) string
	Connector          connector.Connector
}

func NewExecutor(cluster *kubekeyapiv1alpha1.ClusterSpec, objName string, logger *log.Logger, sourcesDir string, debug, skipCheck, skipPullImages, addImagesRepo, inCluster bool, clientset *kubekeyclientset.Clientset) *Executor {
	return &Executor{
		ObjName:        objName,
		Cluster:        cluster,
		Logger:         logger,
		SourcesDir:     sourcesDir,
		Debug:          debug,
		SkipCheck:      skipCheck,
		SkipPullImages: skipPullImages,
		AddImagesRepo:  addImagesRepo,
		InCluster:      inCluster,
		ClientSet:      clientset,
		Connector:      ssh.NewDialer(),
	}
}

func (executor *Executor) CreateManager() (*manager.Manager, error) {
	mgr := &manager.Manager{}
	defaultCluster, hostGroups, err := executor.Cluster.SetDefaultClusterSpec(executor.InCluster, executor.Logger)
	if err != nil {
		return nil, err
	}
	mgr.AllNodes = hostGroups.All
	mgr.EtcdNodes = hostGroups.Etcd
	mgr.MasterNodes = hostGroups.Master
	mgr.WorkerNodes = hostGroups.Worker
	mgr.K8sNodes = hostGroups.K8s
	mgr.Cluster = defaultCluster
	mgr.ClusterHosts = GenerateHosts(hostGroups, defaultCluster)
	mgr.Connector = executor.Connector
	mgr.WorkDir = GenerateWorkDir(executor.Logger)
	mgr.KsEnable = executor.Cluster.KubeSphere.Enabled
	mgr.KsVersion = executor.Cluster.KubeSphere.Version
	mgr.Logger = executor.Logger
	mgr.Debug = executor.Debug
	mgr.SkipCheck = executor.SkipCheck
	mgr.SkipPullImages = executor.SkipPullImages
	mgr.SourcesDir = executor.SourcesDir
	mgr.AddImagesRepo = executor.AddImagesRepo
	mgr.ObjName = executor.ObjName
	mgr.InCluster = executor.InCluster
	mgr.DeployLocalStorage = executor.DeployLocalStorage
	mgr.ClientSet = executor.ClientSet
	mgr.DownloadCommand = executor.DownloadCommand
	if (executor.Cluster.Kubernetes.ContainerManager == "" || executor.Cluster.Kubernetes.ContainerManager == "docker") && executor.Cluster.Kubernetes.Type != "k3s" {
		mgr.EtcdContainer = true
	}
	return mgr, nil
}

func GenerateHosts(hostGroups *kubekeyapiv1alpha1.HostGroups, cfg *kubekeyapiv1alpha1.ClusterSpec) []string {
	var lbHost string
	hostsList := []string{}

	if cfg.ControlPlaneEndpoint.Address != "" {
		lbHost = fmt.Sprintf("%s  %s", cfg.ControlPlaneEndpoint.Address, cfg.ControlPlaneEndpoint.Domain)
	} else {
		lbHost = fmt.Sprintf("%s  %s", hostGroups.Master[0].InternalAddress, cfg.ControlPlaneEndpoint.Domain)
	}

	for _, host := range cfg.Hosts {
		if host.Name != "" {
			hostsList = append(hostsList, fmt.Sprintf("%s  %s.%s %s", host.InternalAddress, host.Name, cfg.Kubernetes.ClusterName, host.Name))
		}
	}

	hostsList = append(hostsList, lbHost)
	return hostsList
}

func GenerateWorkDir(logger *log.Logger) string {
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		logger.Fatal(errors.Wrap(err, "Failed to get current dir"))
	}
	return fmt.Sprintf("%s/%s", currentDir, kubekeyapiv1alpha1.DefaultPreDir)
}
