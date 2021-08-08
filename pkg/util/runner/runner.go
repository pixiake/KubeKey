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

package runner

import (
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/connector"
	"github.com/pkg/errors"
	"time"
)

type Runner struct {
	Conn  connector.Connection
	Debug bool
	Host  *kubekeyapiv1alpha1.HostCfg
	Index int
}

func (r *Runner) ExecuteCmd(cmd string, retries int, printOutput bool, args ...string) (string, error) {
	if r.Conn == nil {
		return "", errors.New("No ssh connection available")
	}

	var lastErr error
	var lastOutput string

	for _, i := range args {
		if i == "printCmd" {
			fmt.Printf("[%s %s] MSG:\n", r.Host.Name, r.Host.Address)
			fmt.Printf("%s\n", cmd)
		}
	}

retriesLoop:
	for i := retries; i >= 0; i-- {
		output, err := r.Conn.Exec(cmd, r.Host)
		if err != nil {
			if i == 0 {
				lastErr = err
				lastOutput = output
			}
			if retries != 0 {
				time.Sleep(time.Second * 5)
			}
		} else {
			if printOutput && output != "" {
				fmt.Printf("[%s %s] MSG:\n", r.Host.Name, r.Host.Address)
				fmt.Println(output)
			}
			lastErr = err
			lastOutput = output
			break retriesLoop
		}
	}

	return lastOutput, lastErr
}

func (r *Runner) ScpFile(src, dst string) error {
	if r.Conn == nil {
		return errors.New("Runner is not tied to an opened SSH connection")
	}

	err := r.Conn.Scp(src, dst)
	if err != nil {
		if r.Debug {
			fmt.Printf("Push %s to %s:%s   Failed\n", src, r.Host.Address, dst)
			return err
		}
	} else {
		if r.Debug {
			fmt.Printf("Push %s to %s:%s   Done\n", src, r.Host.Address, dst)
		}
	}
	return nil
}
