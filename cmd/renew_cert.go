package cmd

import (
	"github.com/kubesphere/kubekey/pkg/cluster/certs"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/spf13/cobra"
)

var renewClusterCertsCmd = &cobra.Command{
	Use:   "renew",
	Short: "renew a cluster certs",
	Run: func(cmd *cobra.Command, args []string) {
		logger := util.InitLogger(opt.Verbose)
		_ = certs.RenewClusterCerts(opt.ClusterCfgFile, logger, opt.Verbose)
	},
}

func init() {
	certsCmd.AddCommand(renewClusterCertsCmd)

	renewClusterCertsCmd.Flags().StringVarP(&opt.ClusterCfgFile, "filename", "f", "", "Path to a configuration file")
}
