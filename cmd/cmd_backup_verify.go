package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zbiljic/fget/pkg/fbackup"
)

type backupVerifyFlags struct {
	Backup string
	Deep   bool
}

var backupVerifyCmdFlags backupVerifyFlags

var backupVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify backup artifacts and metadata",
	Args:  cobra.NoArgs,
	RunE:  runBackupVerify,
}

func init() {
	backupCmd.AddCommand(backupVerifyCmd)
	backupVerifyCmd.Flags().StringVar(&backupVerifyCmdFlags.Backup, "backup", "", "Backup directory")
	backupVerifyCmd.Flags().BoolVar(&backupVerifyCmdFlags.Deep, "deep", false, "Verify Git bundles and tar contents")
}

func runBackupVerify(cmd *cobra.Command, _ []string) error {
	if strings.TrimSpace(backupVerifyCmdFlags.Backup) == "" {
		return errors.New("--backup is required")
	}
	return fbackup.Verify(cmd.Context(), backupVerifyCmdFlags.Backup, backupVerifyCmdFlags.Deep)
}
