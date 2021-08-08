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

package util

import (
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

func NewClient(config string) (*kubernetes.Clientset, error) {
	var kubeconfig string
	if config != "" {
		config, err := filepath.Abs(config)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to look up current directory")
		}
		kubeconfig = config
	} else {
		kubeconfig = filepath.Join(homeDir(), ".kube", "config")
	}
	// use the current context in kubeconfig
	configCluster, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(configCluster)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func NewDynamicClient(config string) (*rest.Config, error) {
	var kubeconfig string
	if config != "" {
		config, err := filepath.Abs(config)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to look up current directory")
		}
		kubeconfig = config
	} else {
		kubeconfig = filepath.Join(homeDir(), ".kube", "config")
	}
	// use the current context in kubeconfig
	configCluster, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	return configCluster, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}
