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

package ssh

import (
	"github.com/kubesphere/kubekey/pkg/connector"
	"sync"
	"time"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
)

type Dialer struct {
	lock        sync.Mutex
	connections map[int]connector.Connection
}

func NewDialer() *Dialer {
	return &Dialer{
		connections: make(map[int]connector.Connection),
	}
}

func (dialer *Dialer) Connect(host kubekeyapiv1alpha1.HostCfg) (connector.Connection, error) {
	var err error

	dialer.lock.Lock()
	defer dialer.lock.Unlock()

	conn, _ := dialer.connections[host.Index]

	opts := Cfg{
		Username:   host.User,
		Port:       host.Port,
		Address:    host.Address,
		Password:   host.Password,
		PrivateKey: host.PrivateKey,
		KeyFile:    host.PrivateKeyPath,
		Timeout:    30 * time.Second,
	}
	conn, err = NewConnection(opts)
	if err != nil {
		return nil, err
	}
	dialer.connections[host.Index] = conn

	return conn, nil
}
