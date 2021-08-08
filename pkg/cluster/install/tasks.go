package install

import (
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/bootstrap/configuration"
	"github.com/kubesphere/kubekey/pkg/k3s"
	k3spreinstall "github.com/kubesphere/kubekey/pkg/k3s/preinstall"
	"github.com/kubesphere/kubekey/pkg/kubernetes"
	k8spreinstall "github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/loadbalancer"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
)

// Precheck is used to perform the check function.
func Precheck(mgr *manager.Manager) error {
	//Check that the number of Etcd is odd
	if len(mgr.EtcdNodes)%2 == 0 {
		mgr.Logger.Warnln("The number of etcd is even. Please configure it to be odd.")
		return errors.New("the number of etcd is even")
	}

	if !mgr.SkipCheck {
		if err := mgr.RunTaskOnAllNodes(k8spreinstall.PrecheckNodes, true); err != nil {
			return err
		}
		k8spreinstall.PrecheckConfirm(mgr)
	}
	return nil
}

// DownloadBinaries is used to download kubernetes' binaries.
func DownloadBinaries(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init nodes", metav1.Now(), metav1.Now(), false, 1); err != nil {
			return err
		}
	}

	mgr.Logger.Infoln("Downloading Installation Files")
	cfg := mgr.Cluster
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return errors.Wrap(err, "Failed to get current directory")
	}

	var kubeVersion string
	if cfg.Kubernetes.Version == "" {
		kubeVersion = kubekeyapiv1alpha1.DefaultKubeVersion
	} else {
		kubeVersion = cfg.Kubernetes.Version
	}

	archMap := make(map[string]bool)
	for _, host := range mgr.Cluster.Hosts {
		switch host.Arch {
		case "amd64":
			archMap["amd64"] = true
		case "arm64":
			archMap["arm64"] = true
		default:
			return errors.New(fmt.Sprintf("Unsupported architecture: %s", host.Arch))
		}
	}

	for arch := range archMap {
		binariesDir := fmt.Sprintf("%s/%s/%s/%s", currentDir, kubekeyapiv1alpha1.DefaultPreDir, kubeVersion, arch)
		if err := util.CreateDir(binariesDir); err != nil {
			return errors.Wrap(err, "Failed to create download target dir")
		}

		switch mgr.Cluster.Kubernetes.Type {
		case "k3s":
			if err := k3spreinstall.FilesDownloadHTTP(mgr, binariesDir, kubeVersion, arch); err != nil {
				return err
			}
		default:
			if err := k8spreinstall.FilesDownloadHTTP(mgr, binariesDir, kubeVersion, arch); err != nil {
				return err
			}
		}
	}
	return nil
}

// InitOS is uesed to initialize the operating system. shuch as: override hostname, configuring kernel parameters, etc.
func InitOS(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Configuring operating system ...")
	return mgr.RunTaskOnAllNodes(configuration.InitOsOnNode, true)
}

// PrePullImages is used to perform PullImages function.
func PrePullImages(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Pull images", metav1.Now(), metav1.Now(), false, 2); err != nil {
			return err
		}
	}

	if !mgr.SkipPullImages {
		mgr.Logger.Infoln("Start to download images on all nodes")
		if err := mgr.RunTaskOnAllNodes(k8spreinstall.PullImages, true); err != nil {
			return err
		}
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Pull images", mgr.Conditions[1].StartTime, metav1.Now(), true, 2); err != nil {
			return err
		}
	}
	return nil
}

// GetClusterStatus is used to fetch status and info from cluster.
func GetClusterStatus(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init control plane", metav1.Now(), metav1.Now(), false, 4); err != nil {
			return err
		}
	}

	mgr.Logger.Infoln("Get cluster status")

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		return mgr.RunTaskOnMasterNodes(k3s.GetClusterStatus, false)
	default:
		return mgr.RunTaskOnMasterNodes(kubernetes.GetClusterStatus, false)
	}

}

// InstallKubeBinaries is used to install kubernetes' binaries to os' PATH.
func InstallKubeBinaries(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Installing kube binaries")

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		return mgr.RunTaskOnK8sNodes(k3s.InstallKubeBinaries, true)
	default:
		return mgr.RunTaskOnK8sNodes(kubernetes.InstallKubeBinaries, true)
	}
}

// InitKubernetesCluster is used to init a new cluster.
func InitKubernetesCluster(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Initializing kubernetes cluster")

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		return mgr.RunTaskOnMasterNodes(k3s.InitK3sCluster, true)
	default:
		return mgr.RunTaskOnMasterNodes(kubernetes.InitKubernetesCluster, true)
	}
}

// JoinNodesToCluster is used to join node to Cluster.
func JoinNodesToCluster(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Join nodes", metav1.Now(), metav1.Now(), false, 5); err != nil {
			return err
		}
	}

	mgr.Logger.Infoln("Joining nodes to cluster")

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		if err := mgr.RunTaskOnK8sNodes(k3s.JoinNodesToCluster, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnK8sNodes(k3s.AddLabelsForNodes, true); err != nil {
			return err
		}
	default:
		if err := mgr.RunTaskOnK8sNodes(kubernetes.JoinNodesToCluster, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnK8sNodes(kubernetes.AddLabelsForNodes, true); err != nil {
			return err
		}
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Join nodes", mgr.Conditions[4].StartTime, metav1.Now(), true, 5); err != nil {
			return err
		}
	}

	return nil
}

// InstallInternalLoadbalancer is used to install a internal load balancer to cluster.
func InstallInternalLoadbalancer(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Install internal load balancer to cluster")

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		if err := mgr.RunTaskOnWorkerNodes(loadbalancer.DeployHaproxy, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnWorkerNodes(k3s.UpdateK3sConfig, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnK8sNodes(kubernetes.UpdateHostsFile, true); err != nil {
			return err
		}

	default:
		if err := mgr.RunTaskOnWorkerNodes(loadbalancer.DeployHaproxy, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnK8sNodes(kubernetes.UpdateKubeletConfig, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnMasterNodes(kubernetes.UpdateKubeproxyConfig, true); err != nil {
			return err
		}
		if err := mgr.RunTaskOnK8sNodes(kubernetes.UpdateHostsFile, true); err != nil {
			return err
		}
	}
	return nil
}
