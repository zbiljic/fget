package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zbiljic/fget/pkg/fbackup"
)

func TestBackupRestoreValidatesRequiredFlags(t *testing.T) {
	old := backupRestoreCmdFlags
	t.Cleanup(func() { backupRestoreCmdFlags = old })
	command := &cobra.Command{}
	backupRestoreCmdFlags = backupRestoreFlags{}
	if err := runBackupRestore(command, nil); err == nil || !strings.Contains(err.Error(), "--backup is required") {
		t.Fatalf("runBackupRestore() error = %v", err)
	}
	backupRestoreCmdFlags = backupRestoreFlags{Backup: "backup"}
	if err := runBackupRestore(command, nil); err == nil || !strings.Contains(err.Error(), "--destination is required") {
		t.Fatalf("runBackupRestore() error = %v", err)
	}
}

func TestBackupRestorePrintsDeterministicCountsOnSuccessAndFailure(t *testing.T) {
	oldFlags, oldFn := backupRestoreCmdFlags, backupRestoreFn
	t.Cleanup(func() { backupRestoreCmdFlags, backupRestoreFn = oldFlags, oldFn })
	var stdout, stderr bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	backupRestoreCmdFlags = backupRestoreFlags{Backup: "backup", Destination: "destination"}
	backupRestoreFn = func(_ context.Context, options fbackup.RestoreOptions) (fbackup.RestoreReport, error) {
		if options.DryRun {
			return fbackup.RestoreReport{Planned: []string{"a", "b"}}, nil
		}
		return fbackup.RestoreReport{Restored: []string{"a"}, Skipped: []string{"b"}, Failed: []string{"c"}}, errors.New("one failed")
	}
	if err := runBackupRestore(command, nil); err == nil {
		t.Fatal("expected aggregate failure")
	}
	if got := stdout.String(); !strings.Contains(got, "restored: 1 skipped: 1 failed: 1") {
		t.Fatalf("failure output = %q", got)
	}
	stdout.Reset()
	backupRestoreCmdFlags.DryRun = true
	if err := runBackupRestore(command, nil); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "a\nb\nplanned: 2\nrestored: 0 skipped: 0 failed: 0\n" {
		t.Fatalf("dry-run output = %q", got)
	}
}
