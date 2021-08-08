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

package config

import (
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/lithammer/dedent"
)

var (
	// KubeletServiceTempl defines the template of kubelete service for systemd.
	KubeletServiceTempl = template.Must(template.New("kubeletService").Parse(
		dedent.Dedent(`[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=http://kubernetes.io/docs/

[Service]
ExecStart=/usr/local/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
    `)))

	// KubeletEnvTempl defines the template of kubelet's Env for the kubelet's systemd service.
	KubeletEnvTempl = template.Must(template.New("kubeletEnv").Parse(
		dedent.Dedent(`# Note: This dropin only works with kubeadm and kubelet v1.11+
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
# This is a file that "kubeadm init" and "kubeadm join" generate at runtime, populating the KUBELET_KUBEADM_ARGS variable dynamically
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
# This is a file that the user can use for overrides of the kubelet args as a last resort. Preferably, the user should use
# the .NodeRegistration.KubeletExtraArgs object in the configuration files instead. KUBELET_EXTRA_ARGS should be sourced from this file.
EnvironmentFile=-/etc/default/kubelet
Environment="KUBELET_EXTRA_ARGS=--node-ip={{ .NodeIP }} --hostname-override={{ .Hostname }} {{ if .ContainerRuntime }}--network-plugin=cni{{ end }}"
ExecStart=
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
    `)))
)

// GenerateKubeletService is used to generate kubelet's service content for systemd.
func GenerateKubeletService() (string, error) {
	return util.Render(KubeletServiceTempl, util.Data{})
}

// GenerateKubeletEnv is used to generate the env content of kubelet's service for systemd.
func GenerateKubeletEnv(node *kubekeyapiv1alpha1.HostCfg) (string, error) {
	var containerRuntime string

	return util.Render(KubeletEnvTempl, util.Data{
		"NodeIP":           node.InternalAddress,
		"Hostname":         node.Name,
		"ContainerRuntime": containerRuntime,
	})
}
