package preinstall

import (
	"strings"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/images"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	versionutil "k8s.io/apimachinery/pkg/util/version"
)

// PullImages defines the list of images that need to be downloaded in advance and downloads them.
func PullImages(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	i := images.Images{}
	i.Images = []images.Image{
		GetImage(mgr, "etcd"),
		GetImage(mgr, "pause"),
		GetImage(mgr, "kube-apiserver"),
		GetImage(mgr, "kube-controller-manager"),
		GetImage(mgr, "kube-scheduler"),
		GetImage(mgr, "kube-proxy"),
		GetImage(mgr, "coredns"),
		GetImage(mgr, "k8s-dns-node-cache"),
		GetImage(mgr, "calico-kube-controllers"),
		GetImage(mgr, "calico-cni"),
		GetImage(mgr, "calico-node"),
		GetImage(mgr, "calico-flexvol"),
		GetImage(mgr, "cilium"),
		GetImage(mgr, "operator-generic"),
		GetImage(mgr, "flannel"),
		GetImage(mgr, "kubeovn"),
		GetImage(mgr, "haproxy"),
	}
	if err := i.PullImages(mgr, node); err != nil {
		return err
	}
	return nil
}

// GetImage defines the list of all images and gets image object by name.
func GetImage(mgr *manager.Manager, name string) images.Image {
	var image images.Image
	var pauseTag string

	cmp, err := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version).Compare("v1.18.0")
	if err != nil {
		mgr.Logger.Fatal("Failed to compare version: %v", err)
	}
	if (cmp == 0 || cmp == 1) || (mgr.Cluster.Kubernetes.ContainerManager != "" && mgr.Cluster.Kubernetes.ContainerManager != "docker") {
		pauseTag = "3.2"
	} else {
		pauseTag = "3.1"
	}

	ImageList := map[string]images.Image{
		"pause":                   {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "pause", Tag: pauseTag, Group: kubekeyapiv1alpha1.K8s, Enable: true},
		"kube-apiserver":          {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "kube-apiserver", Tag: mgr.Cluster.Kubernetes.Version, Group: kubekeyapiv1alpha1.Master, Enable: true},
		"kube-controller-manager": {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "kube-controller-manager", Tag: mgr.Cluster.Kubernetes.Version, Group: kubekeyapiv1alpha1.Master, Enable: true},
		"kube-scheduler":          {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "kube-scheduler", Tag: mgr.Cluster.Kubernetes.Version, Group: kubekeyapiv1alpha1.Master, Enable: true},
		"kube-proxy":              {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "kube-proxy", Tag: mgr.Cluster.Kubernetes.Version, Group: kubekeyapiv1alpha1.K8s, Enable: true},
		"etcd":                    {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "etcd", Tag: kubekeyapiv1alpha1.DefaultEtcdVersion, Group: kubekeyapiv1alpha1.Etcd, Enable: true},
		// network
		"coredns":                 {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "coredns", Repo: "coredns", Tag: "1.6.9", Group: kubekeyapiv1alpha1.K8s, Enable: true},
		"k8s-dns-node-cache":      {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "k8s-dns-node-cache", Tag: "1.15.12", Group: kubekeyapiv1alpha1.K8s, Enable: true},
		"calico-kube-controllers": {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "calico", Repo: "kube-controllers", Tag: kubekeyapiv1alpha1.DefaultCalicoVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "calico")},
		"calico-cni":              {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "calico", Repo: "cni", Tag: kubekeyapiv1alpha1.DefaultCalicoVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "calico")},
		"calico-node":             {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "calico", Repo: "node", Tag: kubekeyapiv1alpha1.DefaultCalicoVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "calico")},
		"calico-flexvol":          {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "calico", Repo: "pod2daemon-flexvol", Tag: kubekeyapiv1alpha1.DefaultCalicoVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "calico")},
		"calico-typha":            {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "calico", Repo: "typha", Tag: kubekeyapiv1alpha1.DefaultCalicoVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "calico") && len(mgr.K8sNodes) > 50},
		"flannel":                 {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: kubekeyapiv1alpha1.DefaultKubeImageNamespace, Repo: "flannel", Tag: kubekeyapiv1alpha1.DefaultFlannelVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "flannel")},
		"cilium":                  {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "cilium", Repo: "cilium", Tag: kubekeyapiv1alpha1.DefaultCiliumVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "cilium")},
		"operator-generic":        {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "cilium", Repo: "operator-generic", Tag: kubekeyapiv1alpha1.DefaultCiliumVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "cilium")},
		"kubeovn":                 {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "kubeovn", Repo: "kube-ovn", Tag: kubekeyapiv1alpha1.DefaultKubeovnVersion, Group: kubekeyapiv1alpha1.K8s, Enable: strings.EqualFold(mgr.Cluster.Network.Plugin, "kubeovn")},
		// storage
		"provisioner-localpv": {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "openebs", Repo: "provisioner-localpv", Tag: "2.10.1", Group: kubekeyapiv1alpha1.Worker, Enable: false},
		"linux-utils":         {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "openebs", Repo: "linux-utils", Tag: "2.10.0", Group: kubekeyapiv1alpha1.Worker, Enable: false},

		// load balancer
		"haproxy": {RepoAddr: mgr.Cluster.Registry.PrivateRegistry, Namespace: "library", Repo: "haproxy", Tag: "2.3", Group: kubekeyapiv1alpha1.Worker, Enable: mgr.Cluster.ControlPlaneEndpoint.IsInternalLBEnabled()},
	}

	image = ImageList[name]
	return image
}
