package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zbiljic/fget/pkg/fbackup"
)

func TestRunBackupCreateAndVerifyCommands(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "audit.json")
	repositoryPath := filepath.Join(root, "repository")
	if err := os.Mkdir(repositoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fbackup.Manifest{Version: fbackup.ManifestVersion, Repositories: []fbackup.RepositoryEntry{
		{ID: "full", Path: repositoryPath, Classification: fbackup.ClassificationFull},
	}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "backup")
	originalCreate := backupCreateCmdFlags
	originalVerify := backupVerifyCmdFlags
	t.Cleanup(func() { backupCreateCmdFlags = originalCreate; backupVerifyCmdFlags = originalVerify })
	backupCreateCmdFlags = backupCreateFlags{Manifest: manifestPath, Destination: destination}
	command := &cobra.Command{}
	command.SetContext(context.Background())
	var stderr bytes.Buffer
	command.SetErr(&stderr)
	if err := runBackupCreate(command, nil); err != nil {
		t.Fatal(err)
	}
	backupVerifyCmdFlags = backupVerifyFlags{Backup: destination, Deep: true}
	if err := runBackupVerify(command, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunBackupCreateFlagValidation(t *testing.T) {
	original := backupCreateCmdFlags
	t.Cleanup(func() { backupCreateCmdFlags = original })
	command := &cobra.Command{}
	command.SetContext(context.Background())
	cases := []backupCreateFlags{
		{Destination: "backup"},
		{Manifest: "manifest.json"},
	}
	for _, flags := range cases {
		backupCreateCmdFlags = flags
		if err := runBackupCreate(command, nil); err == nil {
			t.Fatalf("runBackupCreate(%+v) succeeded", flags)
		}
	}
}
