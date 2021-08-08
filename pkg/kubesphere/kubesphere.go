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

package kubesphere

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"

	"github.com/kubesphere/kubekey/pkg/plugins/storage"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
)

var stopChan = make(chan string, 1)

func DeployKubeSphere(mgr *manager.Manager) error {

	if mgr.Cluster.KubeSphere.Enabled {
		mgr.Logger.Infoln("Deploying KubeSphere ...")
		if err := mgr.RunTaskOnMasterNodes(deployKubeSphere, true); err != nil {
			return err
		}
		if err := ResultNotes(mgr.InCluster); err != nil {
			return err
		}
	}

	return nil
}

func deployKubeSphere(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"mkdir -p /etc/kubernetes/addons\"", 1, false)

		if err := DeployKubeSphereStep(mgr, node); err != nil {
			return err
		}
	}

	return nil
}

func DeployKubeSphereStep(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	ksVersion := mgr.Cluster.KubeSphere.Version
	fmt.Println(ksVersion)
	switch ksVersion {
	case "v2.1.1":
		err := mgr.Runner.ScpFile(fmt.Sprintf("%s/%s/%s/%s", mgr.WorkDir, mgr.Cluster.Kubernetes.Version, node.Arch, "helm2"), fmt.Sprintf("%s/%s", "/tmp/kubekey", "helm2"))
		if err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to sync helm2"))
		}
		_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"cp /tmp/kubekey/helm2  /usr/local/bin/helm2  && chmod +x /usr/local/bin/helm2\""), 1, false)
		if err1 != nil {
			return errors.Wrap(errors.WithStack(err1), fmt.Sprintf("Failed to sync helm2"))
		}
		_, err2 := mgr.Runner.ExecuteCmd(`cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tiller
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tiller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: tiller
    namespace: kube-system
EOF
`, 5, true)
		if err2 != nil {
			return errors.Wrap(errors.WithStack(err2), fmt.Sprintf("Failed to create helm rbac"))
		}
		var tillerRepo string
		if mgr.Cluster.Registry.PrivateRegistry != "" {
			tillerRepo = fmt.Sprintf("%s/kubesphere", mgr.Cluster.Registry.PrivateRegistry)
		} else {
			tillerRepo = "kubesphere"
		}
		_, err3 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/helm2 init --service-account=tiller --skip-refresh --tiller-image=%s/tiller:v2.16.9 --wait\"", tillerRepo), 3, true)
		if err3 != nil {
			return errors.Wrap(errors.WithStack(err3), fmt.Sprintf("Failed to sync helm2"))
		}

		if err := generateKubeSphereManifests(mgr, ksVersion); err != nil {
			return err
		}
	case "v3.0.0":
		if err := generateKubeSphereManifests(mgr, ksVersion); err != nil {
			return err
		}
	case "v3.1.0":
		if err := generateKubeSphereManifests(mgr, ksVersion); err != nil {
			return err
		}
	case "v3.1.1", "latest":
		if err := generateKubeSphereManifests(mgr, ksVersion); err != nil {
			return err
		}
	default:
		if strings.HasPrefix(ksVersion, "nightly-") {
			if err := generateKubeSphereManifests(mgr, ksVersion); err != nil {
				return err
			}
		}
	}

	var addrList []string
	for _, host := range mgr.EtcdNodes {
		addrList = append(addrList, host.InternalAddress)
	}
	etcdendpoint := strings.Join(addrList, ",")
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo /bin/sh -c \"sed -i '/endpointIps/s/\\:.*/\\: %s/g' /etc/kubernetes/addons/kubesphere.yaml\"", etcdendpoint), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to update etcd endpoint"))
	}

	if mgr.Cluster.Registry.PrivateRegistry != "" {
		PrivateRegistry := strings.Replace(mgr.Cluster.Registry.PrivateRegistry, "/", "\\/", -1)
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo /bin/sh -c \"sed -i '/local_registry/s/\\:.*/\\: %s/g' /etc/kubernetes/addons/kubesphere.yaml\"", PrivateRegistry), 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to add private registry: %s", mgr.Cluster.Registry.PrivateRegistry))
		}
	} else {
		if _, err := mgr.Runner.ExecuteCmd("sudo /bin/sh -c \"sed -i '/local_registry/d' /etc/kubernetes/addons/kubesphere.yaml\"", 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to remove private registry"))
		}
	}

	_, ok := mirrorVersionList[ksVersion]
	if ok && (os.Getenv("KKZONE") == "cn" || os.Getenv("KKZONE") == "" || mgr.Cluster.Registry.PrivateRegistry == "hub.oepkgs.net") {
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo /bin/sh -c \"sed -i '/zone/s/\\:.*/\\: %s/g' /etc/kubernetes/addons/kubesphere.yaml\"", "cn"), 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to add private registry: %s", mgr.Cluster.Registry.PrivateRegistry))
		}
	} else {
		if _, err := mgr.Runner.ExecuteCmd("sudo /bin/sh -c \"sed -i '/zone/d' /etc/kubernetes/addons/kubesphere.yaml\"", 2, false); err != nil {
			return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to remove private registry"))
		}
	}

	_, err3 := mgr.Runner.ExecuteCmd(`cat <<EOF | /usr/local/bin/kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: kubesphere-system
---
apiVersion: v1
kind: Namespace
metadata:
  name: kubesphere-monitoring-system
EOF
`, 5, true)
	if err3 != nil {
		return errors.Wrap(errors.WithStack(err3), "Failed to create namespace: kubesphere-system")
	}

	caFile := "/etc/ssl/etcd/ssl/ca.pem"
	certFile := fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s.pem", mgr.EtcdNodes[0].Name)
	keyFile := fmt.Sprintf("/etc/ssl/etcd/ssl/node-%s-key.pem", mgr.EtcdNodes[0].Name)
	if output, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl -n kubesphere-monitoring-system create secret generic kube-etcd-client-certs --from-file=etcd-client-ca.crt=%s --from-file=etcd-client.crt=%s --from-file=etcd-client.key=%s\"", caFile, certFile, keyFile), 1, true); err != nil {
		if !strings.Contains(output, "AlreadyExists") {
			return err
		}
	}

	deployKubesphereCmd := "sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/addons/kubesphere.yaml --force\""

	if _, err := mgr.Runner.ExecuteCmd(deployKubesphereCmd, 10, true); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to deploy /etc/kubernetes/addons/kubesphere.yaml")
	}

	go CheckKubeSphereStatus(mgr)
	return nil
}

func generateKubeSphereManifests(mgr *manager.Manager, version string) error {
	kubesphereYaml, err := GenerateKubeSphereYaml(mgr.Cluster.Registry.PrivateRegistry, version)
	if err != nil {
		return err
	}
	kubesphereYamlBase64 := base64.StdEncoding.EncodeToString([]byte(kubesphereYaml))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/kubernetes/addons/kubesphere.yaml\"", kubesphereYamlBase64), 2, false); err != nil {
		return errors.Wrap(err, "Failed to generate kubesphere manifests")
	}
	ConfigurationBase64 := base64.StdEncoding.EncodeToString([]byte(mgr.Cluster.KubeSphere.Configurations))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d >> /etc/kubernetes/addons/kubesphere.yaml\"", ConfigurationBase64), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubesphere manifests")
	}
	return nil
}

func CheckKubeSphereStatus(mgr *manager.Manager) {
	for i := 180; i > 0; i-- {
		time.Sleep(10 * time.Second)
		_, err := mgr.Runner.ExecuteCmd(
			"sudo -E /bin/sh -c \"/usr/local/bin/kubectl exec -n kubesphere-system $(kubectl get pod -n kubesphere-system -l app=ks-install -o jsonpath='{.items[0].metadata.name}') -- ls /kubesphere/playbooks/kubesphere_running\"", 0, false,
		)
		if err == nil {
			output, err := mgr.Runner.ExecuteCmd(
				"sudo -E /bin/sh -c \"/usr/local/bin/kubectl exec -n kubesphere-system $(kubectl get pod -n kubesphere-system -l app=ks-install -o jsonpath='{.items[0].metadata.name}') -- cat /kubesphere/playbooks/kubesphere_running\"", 2, false,
			)
			if err == nil && output != "" {
				stopChan <- output
				break
			}
		}
	}
	stopChan <- ""
}

func ResultNotes(incluster bool) error {
	var (
		position = 1
		notes    = "Please wait for the installation to complete: "
	)
	fmt.Print("\n")
	if incluster {
		fmt.Println("Please wait for the installation to complete ...")
	}
Loop:
	for {
		select {
		case result := <-stopChan:
			if !incluster {
				fmt.Printf("\033[%dA\033[K", position)
			}
			if result == "" {
				return errors.New("KubeSphere startup timeout.")
			}
			fmt.Println(result)
			break Loop
		default:
			if !incluster {
				for i := 0; i < 10; i++ {
					if i < 5 {
						fmt.Printf("\033[%dA\033[K", position)

						output := fmt.Sprintf(
							"%s%s%s",
							notes,
							strings.Repeat(" ", i),
							">>--->",
						)

						fmt.Printf("%s \033[K\n", output)
						time.Sleep(time.Duration(200) * time.Millisecond)
					} else {
						fmt.Printf("\033[%dA\033[K", position)

						output := fmt.Sprintf(
							"%s%s%s",
							notes,
							strings.Repeat(" ", 10-i),
							"<---<<",
						)

						fmt.Printf("%s \033[K\n", output)
						time.Sleep(time.Duration(200) * time.Millisecond)
					}
				}
			}
		}
	}
	return nil
}

func DeployLocalVolume(mgr *manager.Manager) error {

	if err := mgr.RunTaskOnMasterNodes(DeployLocalVolumeForCluster, true); err != nil {
		return err
	}

	return nil
}

func DeployLocalVolumeForCluster(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		if err := checkDefaultStorageClass(mgr); err != nil {
			return err
		}
	}
	return nil
}

func checkDefaultStorageClass(mgr *manager.Manager) error {
	output, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get sc --no-headers | grep '(default)' | wc -l\"", 3, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to check default storageClass")
	}
	reg := regexp.MustCompile(`([\d])`)
	defaultStorageClassNum := reg.FindStringSubmatch(output)[0]
	if defaultStorageClassNum == "0" {
		if err := storage.DeployLocalVolume(mgr); err != nil {
			return err
		}
	} else if defaultStorageClassNum != "1" {
		mgr.Logger.Warningln("Default storageClass in cluster is not unique!")
	}
	return nil
}
