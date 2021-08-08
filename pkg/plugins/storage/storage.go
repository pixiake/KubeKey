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

package storage

import (
	"encoding/base64"
	"fmt"
	localvolume "github.com/kubesphere/kubekey/pkg/plugins/storage/local-volume"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
)

func DeployLocalVolume(mgr *manager.Manager) error {

	_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"mkdir -p /etc/kubernetes/addons\"", 1, false)
	localVolumeFile, err := localvolume.GenerateOpenebsManifests(mgr)
	if err != nil {
		return err
	}
	localVolumeFileBase64 := base64.StdEncoding.EncodeToString([]byte(localVolumeFile))
	_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"echo %s | base64 -d > /etc/kubernetes/addons/local-volume.yaml\"", localVolumeFileBase64), 1, false)
	if err1 != nil {
		return errors.Wrap(errors.WithStack(err1), "Failed to generate local-volume manifests")
	}

	_, err2 := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubectl apply -f /etc/kubernetes/addons/local-volume.yaml\"", 5, true)
	if err2 != nil {
		return errors.Wrap(errors.WithStack(err2), "Failed to deploy local-volume.yaml")
	}
	return nil
}
