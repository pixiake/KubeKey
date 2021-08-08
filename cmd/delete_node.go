package cmd

import (
	"github.com/kubesphere/kubekey/pkg/cluster/delete"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/spf13/cobra"
	"strings"
)

var deleteNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "delete a node",
	Run: func(cmd *cobra.Command, args []string) {
		logger := util.InitLogger(opt.Verbose)
		_ = delete.ResetNode(opt.ClusterCfgFile, logger, opt.Verbose, strings.Join(args, ""))
	},
}

func init() {
	deleteCmd.AddCommand(deleteNodeCmd)
	deleteNodeCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
}
