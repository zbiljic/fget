package cmd

import "github.com/spf13/cobra"

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Inspect backup readiness without modifying repositories",
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
