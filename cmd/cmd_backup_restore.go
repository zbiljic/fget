package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zbiljic/fget/pkg/fbackup"
)

type backupRestoreFlags struct {
	Backup      string
	Destination string
	DryRun      bool
}

var (
	backupRestoreCmdFlags           backupRestoreFlags
	backupRestoreFn                 = fbackup.RestoreWithReport
	backupRestoreGitRunnerFactoryFn = func() backupGitRunner { return backupGitCLI{} }
)

var backupRestoreCmd = &cobra.Command{
	Use:   "restore [repo-selector ...]",
	Short: "Restore repositories from verified backup artifacts",
	Args:  cobra.ArbitraryArgs,
	RunE:  runBackupRestore,
}

func init() {
	backupCmd.AddCommand(backupRestoreCmd)
	backupRestoreCmd.Flags().StringVar(&backupRestoreCmdFlags.Backup, "backup", "", "Backup directory")
	backupRestoreCmd.Flags().StringVar(&backupRestoreCmdFlags.Destination, "destination", "", "Empty restore destination root")
	backupRestoreCmd.Flags().BoolVar(&backupRestoreCmdFlags.DryRun, "dry-run", false, "Print the restore plan without writing or contacting remotes")
}

func runBackupRestore(cmd *cobra.Command, args []string) error {
	flags := backupRestoreCmdFlags
	if strings.TrimSpace(flags.Backup) == "" {
		return errors.New("--backup is required")
	}
	if strings.TrimSpace(flags.Destination) == "" {
		return errors.New("--destination is required")
	}
	report, err := backupRestoreFn(cmd.Context(), fbackup.RestoreOptions{
		Backup: flags.Backup, Destination: flags.Destination, RepositoryIDs: args,
		DryRun:    flags.DryRun,
		GitRunner: backupRestoreGitRunnerFactoryFn(),
		Progress:  func(id, status string) { _, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", id, status) },
	})
	if flags.DryRun && err == nil {
		for _, id := range report.Planned {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", id)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "planned: %d\n", len(report.Planned))
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "restored: %d skipped: %d failed: %d\n", len(report.Restored), len(report.Skipped), len(report.Failed))
	return err
}
