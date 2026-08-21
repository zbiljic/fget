package cmd

import "github.com/spf13/cobra"

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Audit, create, and verify repository backups",
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
