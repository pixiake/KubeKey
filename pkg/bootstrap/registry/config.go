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

package registry

import (
	"text/template"

	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/lithammer/dedent"
)

var (
	// RegistryServiceTempl defines the template of registry service for systemd.
	RegistryServiceTempl = template.Must(template.New("registryService").Parse(
		dedent.Dedent(`[Unit]
Description=v2 Registry server for Container
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/registry serve /etc/kubekey/registry/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target

    `)))

	// RegistryConfigTempl defines the template of registry's configuration file.
	RegistryConfigTempl = template.Must(template.New("registryConfig").Parse(
		dedent.Dedent(`version: 0.1
log:
  fields:
    service: registry
storage:
    cache:
        layerinfo: inmemory
    filesystem:
        rootdirectory: /mnt/registry
http:
    addr: :5000
    tls:
      certificate: /etc/kubekey/registry/certs/domain.crt
      key: /etc/kubekey/registry/certs/domain.key

    `)))

	// k3sRegistryConfigTempl defines the template of k3s' registry.
	K3sRegistryConfigTempl = template.Must(template.New("k3sRegistryConfig").Parse(
		dedent.Dedent(`mirrors:
  "dockerhub.kubekey.local:5000":
    endpoint:
      - "https://dockerhub.kubekey.local:5000"
  "docker.io":
    endpoint:
      - "https://dockerhub.kubekey.local:5000"
configs:
  "dockerhub.kubekey.local:5000":
    tls:
      ca_file: "/etc/kubekey/registry/certs/ca.crt"
      insecure_skip_verify: true

    `)))
)

// GenerateRegistryService is used to generate registry's service content for systemd.
func GenerateRegistryService() (string, error) {
	return util.Render(RegistryServiceTempl, util.Data{})
}

// GenerateRegistryConfig is used to generate the configuration file for registry.
func GenerateRegistryConfig() (string, error) {
	return util.Render(RegistryConfigTempl, util.Data{})
}

// GenerateK3sRegistryConfig is used to generate the configuration file for registry.
func GenerateK3sRegistryConfig() (string, error) {
	return util.Render(K3sRegistryConfigTempl, util.Data{})
}
