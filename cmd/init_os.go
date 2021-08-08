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
	"github.com/kubesphere/kubekey/pkg/bootstrap/dependencies"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/spf13/cobra"
)

// osCmd represents the os command
var osCmd = &cobra.Command{
	Use:   "os",
	Short: "Init operating system",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := util.InitLogger(opt.Verbose)
		return dependencies.InitDependencies(opt.ClusterCfgFile, opt.SourcesDir, opt.AddImagesRepo, logger)
	},
}

func init() {
	initCmd.AddCommand(osCmd)
	osCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
	osCmd.Flags().StringVarP(&opt.SourcesDir, "sources", "s", "", "Path to the dependencies' dir")
	osCmd.Flags().BoolVarP(&opt.AddImagesRepo, "add-images-repo", "", false, "Create a local images registry")
}
