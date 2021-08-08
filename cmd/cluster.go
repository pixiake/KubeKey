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
	"github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/cluster/install"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/version"
	"github.com/spf13/cobra"
	"time"
)

// clusterCmd represents the cluster command
var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Create a Kubernetes or KubeSphere cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var ksVersion string
		if opt.Kubesphere && len(args) > 0 {
			ksVersion = args[0]
		} else {
			ksVersion = ""
		}
		logger := util.InitLogger(opt.Verbose)
		return install.CreateCluster(opt.ClusterCfgFile, opt.Kubernetes, ksVersion, logger, opt.Kubesphere, opt.Verbose, opt.SkipCheck, opt.SkipPullImages, opt.InCluster, opt.LocalStorage, opt.DownloadCmd)
	},
}

func init() {
	createCmd.AddCommand(clusterCmd)

	clusterCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
	clusterCmd.Flags().StringVarP(&opt.Kubernetes, "with-kubernetes", "", v1alpha1.DefaultKubeVersion, "Specify a supported version of kubernetes")
	clusterCmd.Flags().BoolVarP(&opt.LocalStorage, "with-local-storage", "", false, "Deploy a local PV provisioner")
	clusterCmd.Flags().BoolVarP(&opt.Kubesphere, "with-kubesphere", "", false, "Deploy a specific version of kubesphere (default v3.1.0)")
	clusterCmd.Flags().BoolVarP(&opt.SkipCheck, "yes", "y", false, "Skip pre-check of the installation")
	clusterCmd.Flags().BoolVarP(&opt.SkipPullImages, "skip-pull-images", "", false, "Skip pre pull images")
	clusterCmd.Flags().StringVarP(&opt.DownloadCmd, "download-cmd", "", "curl -L -o %s %s",
		`The user defined command to download the necessary binary files. The first param '%s' is output path, the second param '%s', is the URL`)

	if err := setValidArgs(clusterCmd); err != nil {
		panic(fmt.Sprintf("Got error with the completion setting"))
	}
}

func setValidArgs(cmd *cobra.Command) (err error) {
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) (
		strings []string, directive cobra.ShellCompDirective) {
		versionArray := []string{"v2.1.1", "v3.0.0", "v3.1.0", "v3.1.1", time.Now().Add(-time.Hour * 24).Format("nightly-20060102")}
		return versionArray, cobra.ShellCompDirectiveNoFileComp
	}

	err = cmd.RegisterFlagCompletionFunc("with-kubernetes", func(cmd *cobra.Command, args []string, toComplete string) (
		strings []string, directive cobra.ShellCompDirective) {
		return version.SupportedK8sVersionList(), cobra.ShellCompDirectiveNoFileComp
	})
	return
}
