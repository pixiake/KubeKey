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
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Create cluster configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		var ksVersion string
		if opt.Kubesphere && len(args) > 0 {
			ksVersion = args[0]
		} else {
			ksVersion = ""
		}
		err := config.GenerateClusterObj(opt.Kubernetes, ksVersion, opt.Name, opt.Kubeconfig, opt.ClusterCfgPath, opt.Kubesphere, opt.FromCluster)
		if err != nil {
			return err
		}
		return err
	},
}

func init() {
	createCmd.AddCommand(configCmd)

	configCmd.Flags().StringVarP(&opt.Name, "name", "", "sample", "Specify a name of cluster object")
	configCmd.Flags().StringVarP(&opt.ClusterCfgPath, "filename", "f", "", "Specify a configuration file path")
	configCmd.Flags().StringVarP(&opt.Kubernetes, "with-kubernetes", "", "", "Specify a supported version of kubernetes")
	configCmd.Flags().BoolVarP(&opt.Kubesphere, "with-kubesphere", "", false, "Deploy a specific version of kubesphere (default v3.1.0)")
	configCmd.Flags().BoolVarP(&opt.FromCluster, "from-cluster", "", false, "Create a configuration based on existing cluster")
	configCmd.Flags().StringVarP(&opt.Kubeconfig, "kubeconfig", "", "", "Specify a kubeconfig file")
}
