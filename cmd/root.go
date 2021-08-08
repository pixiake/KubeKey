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
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

type Options struct {
	Verbose        bool
	Addons         string
	Name           string
	ClusterCfgPath string
	Kubeconfig     string
	FromCluster    bool
	ClusterCfgFile string
	Kubernetes     string
	Kubesphere     bool
	LocalStorage   bool
	SkipCheck      bool
	SkipPullImages bool
	KsVersion      string
	Registry       string
	SourcesDir     string
	AddImagesRepo  bool
	InCluster      bool
	DownloadCmd    string
}

var (
	opt Options
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kk",
	Short: "Kubernetes/KubeSphere Deploy Tool",
	Long: `Deploy a Kubernetes or KubeSphere cluster efficiently, flexibly and easily. There are three scenarios to use KubeKey.
1. Install Kubernetes only
2. Install Kubernetes and KubeSphere together in one command
3. Install Kubernetes first, then deploy KubeSphere on it using https://github.com/kubesphere/ks-installer`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	_ = exec.Command("/bin/bash", "-c", "ulimit -u 65535").Run()
	_ = exec.Command("/bin/bash", "-c", "ulimit -n 65535").Run()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().BoolVar(&opt.InCluster, "in-cluster", false, "Running inside the cluster")
	rootCmd.PersistentFlags().BoolVar(&opt.Verbose, "debug", true, "Print detailed information")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
}
