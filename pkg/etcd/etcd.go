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

package etcd

import (
	"encoding/base64"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/etcd/tmpl"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	certsStr        = make(chan map[string]string)
	certsContent    = map[string]string{}
	etcdCertDir     = "/etc/ssl/etcd/ssl"
	etcdBinDir      = "/usr/local/bin"
	accessAddresses = ""
	peerAddresses   []string
	etcdStatus      = ""
)

const (
	NEWETCDCLUSTER   = "new"
	EXISTETCDCLUSTER = "existing"
)

// GetEtcdStatus Get the status of the ETCD cluster and determine the ETCD information for the current node.
func GetEtcdStatus(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Getting etcd status")

	return mgr.RunTaskOnEtcdNodes(getEtcdStatus, false)
}

func getEtcdStatus(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	output, _ := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f /etc/etcd.env ] && echo 'Configuration file already exists' || echo 'Configuration file will be created'\"", 0, true)
	if strings.TrimSpace(output) == "Configuration file already exists" {
		etcdEnv, _ := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"cat /etc/etcd.env | grep ETCD_NAME\"", 0, true)
		node.EtcdName = etcdEnv[strings.Index(etcdEnv, "=")+1:]
		node.EtcdExist = true
		peerAddresses = append(peerAddresses, fmt.Sprintf("%s=https://%s:2380", node.EtcdName, node.InternalAddress))
		etcdStatus = EXISTETCDCLUSTER
	} else {
		node.EtcdName = fmt.Sprintf("etcd-%s", node.Name)
		node.EtcdExist = false
	}

	if len(peerAddresses) == 0 {
		etcdStatus = NEWETCDCLUSTER
	}
	return nil
}

func GenerateEtcdCerts(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init etcd cluster", metav1.Now(), metav1.Now(), false, 3); err != nil {
			return err
		}
	}

	mgr.Logger.Infoln("Generating etcd certs")

	return mgr.RunTaskOnEtcdNodes(generateCerts, true)
}

func generateCerts(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {

	if (etcdStatus == NEWETCDCLUSTER && mgr.Runner.Index == 0) || (etcdStatus == EXISTETCDCLUSTER && strings.Contains(peerAddresses[0], node.InternalAddress)) {
		certsScript, err := tmpl.GenerateEtcdSslScript(mgr)
		if err != nil {
			return err
		}
		certsScriptBase64 := base64.StdEncoding.EncodeToString([]byte(certsScript))
		_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("echo %s | base64 -d > /tmp/kubekey/make-ssl-etcd.sh && chmod +x /tmp/kubekey/make-ssl-etcd.sh", certsScriptBase64), 1, false)
		if err1 != nil {
			return errors.Wrap(errors.WithStack(err1), "Failed to generate etcd certs script")
		}
		certsOpensslCfg, err := tmpl.GenerateEtcdSslCfg(mgr.Cluster)
		if err != nil {
			return err
		}
		certsOpensslCfgBase64 := base64.StdEncoding.EncodeToString([]byte(certsOpensslCfg))
		_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("echo %s | base64 -d > /tmp/kubekey/openssl.conf", certsOpensslCfgBase64), 1, false)
		if err2 != nil {
			return errors.Wrap(errors.WithStack(err2), "Failed to generate etcd certs script")
		}

		cmd := fmt.Sprintf("mkdir -p %s && /bin/bash -x %s/make-ssl-etcd.sh -f %s/openssl.conf -d %s", etcdCertDir, "/tmp/kubekey", "/tmp/kubekey", etcdCertDir)

		_, err3 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", cmd), 1, false)
		if err3 != nil {
			return errors.Wrap(errors.WithStack(err3), "Failed to generate etcd certs")
		}

		for _, cert := range generateCertsFiles(mgr) {
			certsBase64Cmd := fmt.Sprintf("sudo -E /bin/sh -c \"cat %s/%s | base64 --wrap=0\"", etcdCertDir, cert)
			certsBase64, err4 := mgr.Runner.ExecuteCmd(certsBase64Cmd, 1, false)
			if err4 != nil {
				return errors.Wrap(errors.WithStack(err4), "Failed to get etcd certs content")
			}
			certsContent[cert] = certsBase64
		}

		for i := 1; i <= len(mgr.EtcdNodes)-1; i++ {
			certsStr <- certsContent
		}

	} else {
		_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo mkdir -p %s", etcdCertDir), 1, false)
		for file, cert := range <-certsStr {
			writeCertCmd := fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > %s/%s\"", cert, etcdCertDir, file)
			_, err4 := mgr.Runner.ExecuteCmd(writeCertCmd, 1, false)
			if err4 != nil {
				return errors.Wrap(errors.WithStack(err4), "Failed to write etcd certs content")
			}
		}
	}

	return nil
}

func generateCertsFiles(mgr *manager.Manager) []string {
	var certsList []string
	certsList = append(certsList, "ca.pem")
	certsList = append(certsList, "ca-key.pem")
	for _, host := range mgr.EtcdNodes {
		certsList = append(certsList, fmt.Sprintf("admin-%s.pem", host.Name))
		certsList = append(certsList, fmt.Sprintf("admin-%s-key.pem", host.Name))
		certsList = append(certsList, fmt.Sprintf("member-%s.pem", host.Name))
		certsList = append(certsList, fmt.Sprintf("member-%s-key.pem", host.Name))
	}
	for _, host := range mgr.MasterNodes {
		certsList = append(certsList, fmt.Sprintf("node-%s.pem", host.Name))
		certsList = append(certsList, fmt.Sprintf("node-%s-key.pem", host.Name))
	}
	return certsList
}

func SyncEtcdCertsToMaster(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Synchronizing etcd certs")

	return mgr.RunTaskOnMasterNodes(syncEtcdCertsToMaster, true)
}

func syncEtcdCertsToMaster(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if !node.IsEtcd {
		_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo mkdir -p %s", etcdCertDir), 1, false)
		for file, cert := range certsContent {
			writeCertCmd := fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > %s/%s\"", cert, etcdCertDir, file)
			_, err := mgr.Runner.ExecuteCmd(writeCertCmd, 1, false)
			if err != nil {
				return errors.Wrap(errors.WithStack(err), "Failed to sync etcd certs to master")
			}
		}
	}
	return nil
}

func GenerateEtcdService(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Creating etcd service")

	return mgr.RunTaskOnEtcdNodes(generateEtcdService, true)
}

// Install etcd and etcdctl.
// Starting etcd using binary, when container manager is not docker.
// Starting etcd using docker container, when container manager is docker.
func generateEtcdService(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if err := installEtcdBinaries(mgr, node); err != nil {
		return err
	}

	etcdService, err := tmpl.GenerateEtcdService(node.EtcdName, mgr.EtcdContainer)
	if err != nil {
		return err
	}
	etcdServiceBase64 := base64.StdEncoding.EncodeToString([]byte(etcdService))
	_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/systemd/system/etcd.service\"", etcdServiceBase64), 1, false)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to generate etcd service")
	}

	if mgr.EtcdContainer {
		checkEtcd, _ := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"[ -f /usr/local/bin/etcd ] && echo 'etcd already exists' || echo 'etcd will be installed'\"", 0, true)
		if strings.TrimSpace(checkEtcd) == "etcd will be installed" {
			etcdBin, err := tmpl.GenerateEtcdBinary(mgr, node.EtcdName)
			if err != nil {
				return err
			}
			etcdBinBase64 := base64.StdEncoding.EncodeToString([]byte(etcdBin))
			_, err3 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /usr/local/bin/etcd && chmod +x /usr/local/bin/etcd\"", etcdBinBase64), 1, false)
			if err3 != nil {
				return errors.Wrap(errors.WithStack(err3), "Failed to generate etcd bin")
			}
		}
	}

	var addrList []string
	for _, host := range mgr.EtcdNodes {
		addrList = append(addrList, fmt.Sprintf("https://%s:2379", host.InternalAddress))
	}

	accessAddresses = strings.Join(addrList, ",")

	return nil
}

func installEtcdBinaries(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if !mgr.EtcdContainer {
		tmpDir := "/tmp/kubekey"
		_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"if [ -d %s ]; then rm -rf %s ;fi\" && mkdir -p %s", tmpDir, tmpDir, tmpDir), 1, false)
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to create tmp dir")
		}

		currentDir, err1 := filepath.Abs(filepath.Dir(os.Args[0]))
		if err1 != nil {
			return errors.Wrap(err1, "Failed to get current dir")
		}
		etcdFile := fmt.Sprintf("etcd-%s-linux-%s", kubekeyapiv1alpha1.DefaultEtcdVersion, node.Arch)
		filesDir := fmt.Sprintf("%s/%s/%s/%s", currentDir, kubekeyapiv1alpha1.DefaultPreDir, mgr.Cluster.Kubernetes.Version, node.Arch)
		if err := mgr.Runner.ScpFile(fmt.Sprintf("%s/%s.tar.gz", filesDir, etcdFile), fmt.Sprintf("%s/%s.tar.gz", "/tmp/kubekey", etcdFile)); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to sync etcd tar.gz"))
		}

		installCmd := fmt.Sprintf("tar -zxf %s/%s.tar.gz && cp -f %s/etcd* /usr/local/bin/ && chmod +x /usr/local/bin/etcd* && rm -rf %s", "/tmp/kubekey", etcdFile, etcdFile, etcdFile)
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", installCmd), 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to install etcd binaries."))
		}
	} else {
		getEtcdCtlCmd := fmt.Sprintf("docker run --rm -v /usr/local/bin:/systembindir %s /bin/cp -f /usr/local/bin/etcdctl /systembindir/etcdctl", preinstall.GetImage(mgr, "etcd").ImageName())
		_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", getEtcdCtlCmd), 2, false)
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to get etcdctl")
		}
	}
	return nil
}

func SetupEtcdCluster(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Starting etcd cluster")

	if err := mgr.RunTaskOnEtcdNodes(setupEtcdCluster, false); err != nil {
		return err
	}

	return nil
}

// Configuring and starting etcd cluster.
func setupEtcdCluster(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if node.EtcdExist {
		if err := healthCheck(mgr, node); err != nil {
			return err
		}
		return nil
	}

	peerAddresses = append(peerAddresses, fmt.Sprintf("%s=https://%s:2380", node.EtcdName, node.InternalAddress))

	switch etcdStatus {
	case NEWETCDCLUSTER:
		if err := refreshConfig(mgr, node, peerAddresses, NEWETCDCLUSTER); err != nil {
			return err
		}
	case EXISTETCDCLUSTER:
		if err := refreshConfig(mgr, node, peerAddresses, EXISTETCDCLUSTER); err != nil {
			return err
		}
		joinMemberCmd := fmt.Sprintf("sudo -E /bin/sh -c \"export ETCDCTL_API=2;export ETCDCTL_CERT_FILE='/etc/ssl/etcd/ssl/admin-%s.pem';export ETCDCTL_KEY_FILE='/etc/ssl/etcd/ssl/admin-%s-key.pem';export ETCDCTL_CA_FILE='/etc/ssl/etcd/ssl/ca.pem';%s/etcdctl --endpoints=%s member add %s %s\"", node.Name, node.Name, etcdBinDir, accessAddresses, node.EtcdName, fmt.Sprintf("https://%s:2380", node.InternalAddress))
		fmt.Println(joinMemberCmd)
		_, err := mgr.Runner.ExecuteCmd(joinMemberCmd, 2, true)
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to add etcd member")
		}
		if err := restartEtcd(mgr); err != nil {
			return err
		}
		if err := healthCheck(mgr, node); err != nil {
			return err
		}
		checkMemberCmd := fmt.Sprintf("sudo -E /bin/sh -c \"export ETCDCTL_API=2;export ETCDCTL_CERT_FILE='/etc/ssl/etcd/ssl/admin-%s.pem';export ETCDCTL_KEY_FILE='/etc/ssl/etcd/ssl/admin-%s-key.pem';export ETCDCTL_CA_FILE='/etc/ssl/etcd/ssl/ca.pem';%s/etcdctl --no-sync --endpoints=%s member list\"", node.Name, node.Name, etcdBinDir, accessAddresses)
		memberList, err := mgr.Runner.ExecuteCmd(checkMemberCmd, 2, true)
		if err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to list etcd member")
		}
		if !strings.Contains(memberList, fmt.Sprintf("https://%s:2379", node.InternalAddress)) {
			return errors.Wrap(errors.WithStack(err), "Failed to add etcd member")
		}
	default:
		return errors.New("Failed to get etcd cluster status")
	}

	return nil
}

func RefreshEtcdConfig(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Refreshing etcd configuration")

	return mgr.RunTaskOnEtcdNodes(refreshEtcdConfig, true)
}

func BackupEtcd(mgr *manager.Manager) error {
	mgr.Logger.Infoln("Backup etcd data regularly")

	if err := mgr.RunTaskOnEtcdNodes(backupEtcd, true); err != nil {
		return err
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Init etcd cluster", mgr.Conditions[2].StartTime, metav1.Now(), true, 3); err != nil {
			return err
		}
	}

	return nil
}

// Create etcd backup scripts.
func backupEtcd(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s\"", mgr.Cluster.Kubernetes.EtcdBackupScriptDir), 0, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create etcd backup")
	}
	tmpDir := "/tmp/kubekey"
	etcdBackupScript, _ := tmpl.EtcdBackupScript(mgr, node)
	etcdBackupScriptBase64 := base64.StdEncoding.EncodeToString([]byte(etcdBackupScript))
	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > %s/etcd-backup.sh && chmod +x %s/etcd-backup.sh\"", etcdBackupScriptBase64, tmpDir, tmpDir), 1, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate etcd backup")
	}
	_, err3 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo cp %s/etcd-backup.sh %s &&sudo %s/etcd-backup.sh", tmpDir, mgr.Cluster.Kubernetes.EtcdBackupScriptDir, mgr.Cluster.Kubernetes.EtcdBackupScriptDir), 1, false)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err3), "Failed to run the etcd-backup.sh")
	}
	return nil
}

func refreshEtcdConfig(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {

	if etcdStatus == NEWETCDCLUSTER {
		if err := refreshConfig(mgr, node, peerAddresses, NEWETCDCLUSTER); err != nil {
			return err
		}
		if err := restartEtcd(mgr); err != nil {
			return err
		}
		if err := healthCheck(mgr, node); err != nil {
			return err
		}
	}

	if err := refreshConfig(mgr, node, peerAddresses, EXISTETCDCLUSTER); err != nil {
		return err
	}

	if err := healthCheck(mgr, node); err != nil {
		return err
	}

	return nil
}

func healthCheck(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	checkHealthCmd := fmt.Sprintf("sudo -E /bin/sh -c \"export ETCDCTL_API=2;export ETCDCTL_CERT_FILE='/etc/ssl/etcd/ssl/admin-%s.pem';export ETCDCTL_KEY_FILE='/etc/ssl/etcd/ssl/admin-%s-key.pem';export ETCDCTL_CA_FILE='/etc/ssl/etcd/ssl/ca.pem';%s/etcdctl --endpoints=%s cluster-health | grep -q 'cluster is healthy'\"", node.Name, node.Name, etcdBinDir, accessAddresses)
healthCheckLoop:
	for i := 20; i > 0; i-- {
		_, err := mgr.Runner.ExecuteCmd(checkHealthCmd, 0, false)
		if err != nil {
			fmt.Println("Waiting for etcd to start")
			if i == 1 {
				return errors.Wrap(errors.WithStack(err), "Failed to start etcd cluster")
			}
		} else {
			break healthCheckLoop
		}
		time.Sleep(time.Second * 5)
	}
	return nil
}

func refreshConfig(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg, endpoints []string, state string) error {
	etcdEnv, err := tmpl.GenerateEtcdEnv(node, endpoints, state)
	if err != nil {
		return err
	}
	etcdEnvBase64 := base64.StdEncoding.EncodeToString([]byte(etcdEnv))
	_, err2 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/etcd.env\"", etcdEnvBase64), 1, false)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to generate etcd env")
	}
	return nil
}

func restartEtcd(mgr *manager.Manager) error {
	_, err5 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && systemctl restart etcd && systemctl enable etcd\"", 2, true)
	if err5 != nil {
		return errors.Wrap(errors.WithStack(err5), "Failed to start etcd")
	}
	return nil
}
