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

package upgrade

import (
	"bufio"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/mitchellh/mapstructure"
	"github.com/modood/table"
	"github.com/pkg/errors"
	versionutil "k8s.io/apimachinery/pkg/util/version"
	"os"
	"strings"
)

var versionCheck = map[string]map[string]map[string]bool{
	"v3.1.1": {
		"k8s": {
			"v1.20": true,
			"v1.19": true,
			"v1.18": true,
			"v1.17": true,
			"v1.16": true,
			"v1.15": true,
		},
		"ks": {
			"v3.0.0": true,
			"v3.1.0": true,
		},
	},
	"v3.1.0": {
		"k8s": {
			"v1.20": true,
			"v1.19": true,
			"v1.18": true,
			"v1.17": true,
			"v1.16": true,
			"v1.15": true,
		},
		"ks": {
			"v3.0.0": true,
		},
	},
	"v3.0.0": {
		"k8s": {
			"v1.18": true,
			"v1.17": true,
			"v1.16": true,
			"v1.15": true,
		},
		"ks": {
			"v2.1.1": true,
		},
	},
	"v2.1.1": {
		"k8s": {
			"v1.18": true,
			"v1.17": true,
			"v1.16": true,
			"v1.15": true,
		},
		"ks": {
			"v2.1.1": true,
		},
	},
	"other": {
		"k8s": {
			"v1.20": true,
			"v1.19": true,
			"v1.18": true,
			"v1.17": true,
			"v1.16": true,
			"v1.15": true,
		},
	},
}

func GetClusterInfo(mgr *manager.Manager) error {
	if err := mgr.RunTaskOnAllNodes(preinstall.PrecheckNodes, true); err != nil {
		return err
	}
	var results []preinstall.PrecheckResults
	for node := range preinstall.CheckResults {
		var result preinstall.PrecheckResults
		_ = mapstructure.Decode(preinstall.CheckResults[node], &result)
		results = append(results, result)
	}
	table.OutputA(results)
	fmt.Println()

	warningFlag := false
	cmp, err := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version).Compare("v1.19.0")
	if err != nil {
		mgr.Logger.Fatal("Failed to compare kubernetes version: %v", err)
	}
	if cmp == 0 || cmp == 1 {
		for _, result := range results {
			dockerVersion, err := util.RefineDockerVersion(result.Docker)
			if err != nil {
				mgr.Logger.Fatal("Failed to get docker version: %v", err)
			}
			cmp, err := versionutil.MustParseSemantic(dockerVersion).Compare("20.10.0")
			if err != nil {
				mgr.Logger.Fatal("Failed to compare docker version: %v", err)
			}
			warningFlag = warningFlag || (cmp == -1)
		}

		if warningFlag {
			fmt.Println(`
Warning:

  An old Docker version may cause the failure of upgrade. It is recommended that you upgrade Docker to 20.10+ beforehand.

  Issue: https://github.com/kubernetes/kubernetes/issues/101056`)
			fmt.Print("\n")
		}
	}

	return mgr.RunTaskOnMasterNodes(getClusterInfo, true)
}

func getClusterInfo(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		if err := getKubeConfig(mgr); err != nil {
			return err
		}
		k8sVersionStr, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"cat /etc/kubernetes/manifests/kube-apiserver.yaml | grep 'image:' | rev | cut -d ':' -f1 | rev\"", 1, false)
		if err != nil {
			return errors.Wrap(err, "Failed to get current kube-apiserver version")
		}

		ksVersion, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get deploy -n  kubesphere-system ks-console -o jsonpath='{.metadata.labels.version}'\"", 1, false)
		if err != nil {
			if mgr.Cluster.KubeSphere.Enabled {
				return errors.New("Failed to get kubesphere version")
			} else {
				K8sTargetVersion := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version)
				if _, ok := versionCheck["other"]["k8s"][fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())]; !ok {
					return errors.New(fmt.Sprintf("does not support running on Kubernetes %s", fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())))
				}
			}
		} else {
			if mgr.Cluster.KubeSphere.Enabled {
				var version string
				if strings.Contains(mgr.Cluster.KubeSphere.Version, "latest") || strings.Contains(mgr.Cluster.KubeSphere.Version, "nightly") {
					version = "v3.1.0"
				} else {
					version = mgr.Cluster.KubeSphere.Version
				}
				if _, ok := versionCheck[version]; !ok {
					return errors.New(fmt.Sprintf("Unsupported version: %s", mgr.Cluster.KubeSphere.Version))
				}
				if _, ok := versionCheck[version]["ks"][ksVersion]; !ok {
					return errors.New(fmt.Sprintf("Unsupported upgrade plan: %s to %s", strings.TrimSpace(ksVersion), mgr.Cluster.KubeSphere.Version))
				}
				K8sTargetVersion := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version)
				if _, ok := versionCheck[version]["k8s"][fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())]; !ok {
					return errors.New(fmt.Sprintf("KubeSphere %s does not support running on Kubernetes %s", mgr.Cluster.KubeSphere.Version, fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())))
				}
			} else {
				if _, ok := versionCheck[ksVersion]; !ok {
					return errors.New(fmt.Sprintf("Unsupported version: %s", ksVersion))
				}
				K8sTargetVersion := versionutil.MustParseSemantic(mgr.Cluster.Kubernetes.Version)
				if _, ok := versionCheck[ksVersion]["k8s"][fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())]; !ok {
					return errors.New(fmt.Sprintf("KubeSphere %s does not support running on Kubernetes %s", ksVersion, fmt.Sprintf("v%v.%v", K8sTargetVersion.Major(), K8sTargetVersion.Minor())))
				}
			}
		}

		if err := getNodestatus(mgr); err != nil {
			return err
		}

		fmt.Println("Upgrade Confirmation:")
		fmt.Printf("kubernetes version: %s to %s\n", k8sVersionStr, mgr.Cluster.Kubernetes.Version)
		if mgr.Cluster.KubeSphere.Enabled {
			fmt.Printf("kubesphere version: %s to %s\n\n", ksVersion, mgr.Cluster.KubeSphere.Version)
		} else {
			fmt.Println()
		}

		reader := bufio.NewReader(os.Stdin)
	Loop:
		for {
			fmt.Printf("Continue upgrading cluster? [yes/no]: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				mgr.Logger.Fatal(err)
			}
			input = strings.TrimSpace(input)

			switch input {
			case "yes":
				break Loop
			case "no":
				os.Exit(0)
			default:
				continue
			}
		}
	}
	return nil
}

func getNodestatus(mgr *manager.Manager) error {
	nodestatus, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get node\"", 2, false)
	if err != nil {
		return err
	}
	fmt.Println("Cluster nodes status:")
	fmt.Println(nodestatus + "\n")
	return nil
}

func getKubeConfig(mgr *manager.Manager) error {
	cmd := `
if [ -f $HOME/.kube/config ]; then
   echo 'kubeconfig already exist'
elif [ -f /etc/kubernetes/admin.conf ]; then
   mkdir -p $HOME/.kube && cp /etc/kubernetes/admin.conf $HOME/.kube/config && chown $(id -u):$(id -g) -R $HOME/.kube
else
   echo 'not found kubeconfig'
fi
`
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", cmd), 1, false)
	if err != nil {
		return err
	}

	return nil
}
