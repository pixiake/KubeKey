package storage

import (
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/images"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/util/manager"
)

func prePullStorageImages(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	i := images.Images{}
	i.Images = []images.Image{
		preinstall.GetImage(mgr, "provisioner-localpv"),
		preinstall.GetImage(mgr, "linux-utils"),
		preinstall.GetImage(mgr, "rbd-provisioner"),
		preinstall.GetImage(mgr, "nfs-client-provisioner"),
	}
	if err := i.PullImages(mgr, node); err != nil {
		return err
	}
	return nil
}
