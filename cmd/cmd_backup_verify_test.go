package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunBackupVerifyRequiresBackup(t *testing.T) {
	original := backupVerifyCmdFlags
	t.Cleanup(func() { backupVerifyCmdFlags = original })
	backupVerifyCmdFlags.Backup = ""
	command := &cobra.Command{}
	command.SetContext(context.Background())
	if err := runBackupVerify(command, nil); err == nil {
		t.Fatal("runBackupVerify() succeeded without --backup")
	}
}
