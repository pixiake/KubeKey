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

package manager

import (
	"sync"
	"time"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/connector"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/pkg/errors"

	"github.com/kubesphere/kubekey/pkg/util/runner"
)

const (
	// DefaultCon defineds the number of concurrent.
	DefaultCon = 10
	// Timeout defineds how long the task will take to timeout.
	Timeout = 120
)

// Task defineds the struct of task.
type Task struct {
	Task   func(*Manager) error
	ErrMsg string
	Skip   bool
}

// NodeTask defineds the tasks to be performed on the node.
type NodeTask func(mgr *Manager, node *kubekeyapiv1alpha1.HostCfg) error

// Run is used to control task execution logic.
func (t *Task) Run(mgr *Manager) error {
	backoff := wait.Backoff{
		Steps:    1,
		Duration: 5 * time.Second,
		Factor:   2.0,
	}

	var lastErr error
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		lastErr = t.Task(mgr)
		if lastErr != nil {
			mgr.Logger.Warn("Task failed ...")
			if mgr.Debug {
				mgr.Logger.Warnf("error: %s", lastErr)
			}
			return false, nil
		}
		return true, nil
	})
	if err == wait.ErrWaitTimeout {
		err = lastErr
	}
	return err
}

func (mgr *Manager) runTask(node *kubekeyapiv1alpha1.HostCfg, task NodeTask, index int) error {
	var (
		err  error
		conn connector.Connection
	)

	conn, err = mgr.Connector.Connect(*node)
	if err != nil {
		return errors.Wrapf(err, "Failed to connect to %s", node.Address)
	}

	mgr.Runner = &runner.Runner{
		Conn:  conn,
		Debug: mgr.Debug,
		Host:  node,
		Index: index,
	}

	return task(mgr, node)
}

// RunTaskOnNodes is used to execute tasks on nodes.
func (mgr *Manager) RunTaskOnNodes(nodes []kubekeyapiv1alpha1.HostCfg, task NodeTask, parallel bool) error {
	var err error
	hasErrors := false

	wg := &sync.WaitGroup{}
	ccons := make(chan struct{}, DefaultCon)
	defer close(ccons)

	for i := range nodes {
		mgr := mgr.Copy()
		mgr.Logger = mgr.Logger.WithField("node", nodes[i].Address)

		if parallel {
			ccons <- struct{}{}
			wg.Add(1)
			go func(mgr *Manager, node *kubekeyapiv1alpha1.HostCfg, index int) {
				result := make(chan string)
				defer close(result)
				// generate a timer
				go func(result chan string, ccons chan struct{}) {
					select {
					case <-result:
					case <-time.After(time.Minute * Timeout):
						mgr.Logger.Fatalf("Execute task timeout, Timeout=%ds", Timeout)
					}
					wg.Done()
					<-ccons
				}(result, ccons)

				err = mgr.runTask(node, task, index)
				if err != nil {
					mgr.Logger.Error(err)
					hasErrors = true
				}
				result <- "done"
			}(mgr, &nodes[i], i)
		} else {
			err = mgr.runTask(&nodes[i], task, i)
			if err != nil {
				break
			}
		}
	}

	wg.Wait()

	if hasErrors {
		err = errors.New("interrupted by error")
	}

	return err
}

// RunTaskOnAllNodes is used to execute tasks on all nodes.
func (mgr *Manager) RunTaskOnAllNodes(task NodeTask, parallel bool) error {
	if err := mgr.RunTaskOnNodes(mgr.AllNodes, task, parallel); err != nil {
		return err
	}
	return nil
}

// RunTaskOnEtcdNodes is used to execute tasks on all etcd nodes.
func (mgr *Manager) RunTaskOnEtcdNodes(task NodeTask, parallel bool) error {
	if err := mgr.RunTaskOnNodes(mgr.EtcdNodes, task, parallel); err != nil {
		return err
	}
	return nil
}

// RunTaskOnMasterNodes is used to execute tasks on all master nodes.
func (mgr *Manager) RunTaskOnMasterNodes(task NodeTask, parallel bool) error {
	if err := mgr.RunTaskOnNodes(mgr.MasterNodes, task, parallel); err != nil {
		return err
	}
	return nil
}

// RunTaskOnWorkerNodes is used to execute tasks on all worker nodes.
func (mgr *Manager) RunTaskOnWorkerNodes(task NodeTask, parallel bool) error {
	if err := mgr.RunTaskOnNodes(mgr.WorkerNodes, task, parallel); err != nil {
		return err
	}
	return nil
}

// RunTaskOnK8sNodes is used to execute tasks on all nodes in k8s cluster.
func (mgr *Manager) RunTaskOnK8sNodes(task NodeTask, parallel bool) error {
	if err := mgr.RunTaskOnNodes(mgr.K8sNodes, task, parallel); err != nil {
		return err
	}
	return nil
}
