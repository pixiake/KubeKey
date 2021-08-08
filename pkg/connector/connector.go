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

package connector

import (
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
)

type Connection interface {
	Exec(cmd string, host *kubekeyapiv1alpha1.HostCfg) (stdout string, err error)
	Scp(src, dst string) error
	Close()
}

type Connector interface {
	Connect(host kubekeyapiv1alpha1.HostCfg) (Connection, error)
}
