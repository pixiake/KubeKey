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
	"github.com/kubesphere/kubekey/pkg/cluster/upgrade"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/spf13/cobra"
)

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade your cluster smoothly to a newer version with this command",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := util.InitLogger(opt.Verbose)
		var ksVersion string
		if opt.Kubesphere && len(args) > 0 {
			ksVersion = args[0]
		} else {
			ksVersion = ""
		}
		return upgrade.UpgradeCluster(opt.ClusterCfgFile, opt.Kubernetes, ksVersion, logger, opt.Kubesphere, opt.Verbose, opt.SkipPullImages, opt.DownloadCmd)
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
	upgradeCmd.Flags().StringVarP(&opt.Kubernetes, "with-kubernetes", "", "", "Specify a supported version of kubernetes")
	upgradeCmd.Flags().BoolVarP(&opt.Kubesphere, "with-kubesphere", "", false, "Deploy a specific version of kubesphere (default v3.1.0)")
	upgradeCmd.Flags().BoolVarP(&opt.SkipPullImages, "skip-pull-images", "", false, "Skip pre pull images")
	upgradeCmd.Flags().StringVarP(&opt.DownloadCmd, "download-cmd", "", "curl -L -o %s %s",
		`The user defined command to download the necessary binary files. The first param '%s' is output path, the second param '%s', is the URL`)
}
