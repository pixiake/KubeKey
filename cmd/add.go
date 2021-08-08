package cmd

import "github.com/spf13/cobra"

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add nodes to kubernetes cluster",
}

func init() {
	rootCmd.AddCommand(addCmd)
}
