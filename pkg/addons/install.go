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

package addons

import (
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeycontroller "github.com/kubesphere/kubekey/controllers/kubekey"
	"github.com/kubesphere/kubekey/pkg/addons/charts"
	"github.com/kubesphere/kubekey/pkg/addons/manifests"
	"github.com/kubesphere/kubekey/pkg/kubesphere"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func InstallAddons(mgr *manager.Manager) error {
	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Install addons", metav1.Now(), metav1.Now(), false, 6); err != nil {
			return err
		}
	}
	addonsNum := len(mgr.Cluster.Addons)
	if addonsNum != 0 {
		for index, addon := range mgr.Cluster.Addons {
			if addon.Sources.Chart.Name == "ks-installer" {
				if err := mgr.RunTaskOnMasterNodes(kubesphere.DeployLocalVolumeForCluster, true); err != nil {
					return err
				}
			}
			mgr.Logger.Infof("Installing addon [%v-%v]: %s", addonsNum, index+1, addon.Name)
			if err := installAddon(mgr, &addon, filepath.Join(mgr.WorkDir, fmt.Sprintf("config-%s", mgr.ObjName))); err != nil {
				return err
			}
			if addon.Sources.Chart.Name == "ks-installer" {
				if err := mgr.RunTaskOnMasterNodes(checkKubeSphereStatus, true); err != nil {
					return err
				}
				if err := kubesphere.ResultNotes(mgr.InCluster); err != nil {
					return err
				}
			}
		}
	}

	if mgr.InCluster {
		if err := kubekeycontroller.UpdateClusterConditions(mgr, "Install addons", mgr.Conditions[5].StartTime, metav1.Now(), true, 6); err != nil {
			return err
		}
	}

	return nil
}

func installAddon(mgr *manager.Manager, addon *kubekeyapiv1alpha1.Addon, kubeconfig string) error {
	// install chart
	if addon.Sources.Chart.Name != "" {
		_ = os.Setenv("HELM_NAMESPACE", strings.TrimSpace(addon.Namespace))
		if err := charts.InstallChart(mgr, addon, kubeconfig); err != nil {
			return err
		}
	}

	// install yaml
	if len(addon.Sources.Yaml.Path) != 0 {
		var settings = cli.New()
		p := getter.All(settings)
		for _, yaml := range addon.Sources.Yaml.Path {
			u, _ := url.Parse(yaml)
			_, err := p.ByScheme(u.Scheme)
			if err != nil {
				fp, err := filepath.Abs(yaml)
				if err != nil {
					return errors.Wrap(err, "Failed to look up current directory")
				}
				yamlPaths := []string{fp}
				if err := manifests.InstallYaml(yamlPaths, addon.Namespace, kubeconfig, mgr.Cluster.Kubernetes.Version); err != nil {
					return err
				}
			} else {
				yamlPaths := []string{yaml}
				if err := manifests.InstallYaml(yamlPaths, addon.Namespace, kubeconfig, mgr.Cluster.Kubernetes.Version); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func checkKubeSphereStatus(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		go kubesphere.CheckKubeSphereStatus(mgr)
	}

	return nil
}
