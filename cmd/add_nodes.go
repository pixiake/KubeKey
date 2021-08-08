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
	"github.com/kubesphere/kubekey/pkg/cluster/add"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/spf13/cobra"
)

// addNodesCmd represents the nodes command
var addNodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Add nodes to the cluster according to the new nodes information from the specified configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := util.InitLogger(opt.Verbose)
		return add.AddNodes(opt.ClusterCfgFile, "", "", logger, false, opt.Verbose, opt.SkipCheck, opt.SkipPullImages, opt.InCluster, opt.DownloadCmd)
	},
}

func init() {
	addCmd.AddCommand(addNodesCmd)
	addNodesCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
	addNodesCmd.Flags().BoolVarP(&opt.SkipCheck, "yes", "y", false, "Skip pre-check of the installation")
	addNodesCmd.Flags().BoolVarP(&opt.SkipPullImages, "skip-pull-images", "", false, "Skip pre pull images")
	addNodesCmd.Flags().StringVarP(&opt.DownloadCmd, "download-cmd", "", "curl -L -o %s %s",
		`The user defined command to download the necessary binary files. The first param '%s' is output path, the second param '%s', is the URL`)
}
