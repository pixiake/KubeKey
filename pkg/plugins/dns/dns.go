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

package dns

import (
	"encoding/base64"
	"fmt"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	"strings"
)

func OverrideCorednsService(mgr *manager.Manager) error {
	corednsSvc, err := GenerateCorednsService(mgr)
	if err != nil {
		return err
	}
	corednsSvcgBase64 := base64.StdEncoding.EncodeToString([]byte(corednsSvc))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/kubernetes/coredns-svc.yaml\"", corednsSvcgBase64), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate kubeadm config")
	}
	deleteKubednsSvcCmd := "/usr/local/bin/kubectl delete -n kube-system svc kube-dns"
	_, _ = mgr.Runner.ExecuteCmd(deleteKubednsSvcCmd, 1, true)
	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/coredns-svc.yaml\"", 2, true); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create coredns service")
	}
	return nil
}

func DeployNodelocaldns(mgr *manager.Manager, clusterIP string) error {
	nodelocaldns, err := GenerateNodelocaldnsService(mgr)
	if err != nil {
		return err
	}
	nodelocaldnsBase64 := base64.StdEncoding.EncodeToString([]byte(nodelocaldns))
	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/kubernetes/nodelocaldns.yaml\"", nodelocaldnsBase64), 1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate nodelocaldns manifests")
	}

	if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/nodelocaldns.yaml\"", 5, true); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to create nodelocaldns")
	}

	configMaps, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get cm -n kube-system nodelocaldns\"", 1, false)
	if err != nil && strings.Contains(configMaps, "NotFound") {
		nodelocaldns, err := GenerateNodelocaldnsConfigMap(mgr, clusterIP)
		if err != nil {
			return err
		}
		nodelocaldnsConfigMapBase64 := base64.StdEncoding.EncodeToString([]byte(nodelocaldns))
		if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/kubernetes/nodelocaldnsConfigmap.yaml\"", nodelocaldnsConfigMapBase64), 1, false); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to generate nodelocaldns configmap")
		}

		if _, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/nodelocaldnsConfigmap.yaml\"", 5, true); err != nil {
			return errors.Wrap(errors.WithStack(err), "Failed to create nodelocaldns configmap")
		}
	}
	return nil
}

func CreateClusterDns(mgr *manager.Manager) error {
	var corednsClusterIP string
	services, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get svc -n kube-system coredns\"", 1, false)
	if err != nil {
		if strings.Contains(services, "NotFound") {
			if err := OverrideCorednsService(mgr); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if clusterIP, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl get svc -n kube-system coredns -o jsonpath='{.spec.clusterIP}'\"", 1, false); err != nil {
			return err
		} else {
			corednsClusterIP = strings.TrimSpace(clusterIP)
		}
	}

	if err := DeployNodelocaldns(mgr, corednsClusterIP); err != nil {
		return err
	}

	return nil
}
