package kubeovn

import (
	"fmt"

	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"

	"github.com/kubesphere/kubekey/pkg/util/manager"
)

func LabelNode(mgr *manager.Manager) error {
	_, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl label no -lbeta.kubernetes.io/os=linux kubernetes.io/os=linux --overwrite\"", 2, true)
	if err != nil {
		return fmt.Errorf("failed overwrite node label with error: %v", err)
	}

	_, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl label no -l%s kube-ovn/role=master --overwrite\"", mgr.Cluster.Network.Kubeovn.Label), 2, true)
	if err != nil {
		return fmt.Errorf("failed label kubeovn/role=master in master node with error: %v", err)
	}

	return nil
}

func GenerateSSL(mgr *manager.Manager) error {
	exists, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get secret -n kube-system kube-ovn-tls --ignore-not-found\"", 2, true)
	if err != nil {
		return fmt.Errorf("failed find ovn secret: %v", err)
	}
	if exists != "" {
		return nil
	}
	_, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"docker run --rm -v %s:/etc/ovn %s bash generate-ssl.sh\"", mgr.WorkDir, preinstall.GetImage(mgr, "kubeovn").ImageName()), 2, true)
	if err != nil {
		return fmt.Errorf("failed generate ovn secret: %v", err)
	}

	_, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl create secret generic -n kube-system kube-ovn-tls --from-file=cacert=%s/cacert.pem --from-file=cert=%s/ovn-cert.pem --from-file=key=%s/ovn-privkey.pem\"", mgr.WorkDir, mgr.WorkDir, mgr.WorkDir), 2, true)
	if err != nil {
		return fmt.Errorf("failed create ovn secret: %v", err)
	}

	_, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"rm -rf %s/cacert.pem %s/ovn-cert.pem %s/ovn-privkey.pem %s/ovn-req.pem\"", mgr.WorkDir, mgr.WorkDir, mgr.WorkDir, mgr.WorkDir), 2, true)
	if err != nil {
		return fmt.Errorf("failed delete generated ovn secret file: %v", err)
	}

	return nil
}
