package cmd

import "github.com/spf13/cobra"

var certsCmd = &cobra.Command{
	Use:   "certs",
	Short: "cluster certs",
}

func init() {
	rootCmd.AddCommand(certsCmd)
}
