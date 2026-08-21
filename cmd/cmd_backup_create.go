package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zbiljic/fget/pkg/fbackup"
)

type backupCreateFlags struct {
	Manifest    string
	Destination string
}

var backupCreateCmdFlags backupCreateFlags

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create restartable backup artifacts from an audit manifest",
	Args:  cobra.NoArgs,
	RunE:  runBackupCreate,
}

func init() {
	backupCmd.AddCommand(backupCreateCmd)
	backupCreateCmd.Flags().StringVar(&backupCreateCmdFlags.Manifest, "manifest", "", "Audit manifest JSON file")
	backupCreateCmd.Flags().StringVar(&backupCreateCmdFlags.Destination, "destination", "", "Backup destination directory")
}

func runBackupCreate(cmd *cobra.Command, _ []string) error {
	flags := backupCreateCmdFlags
	if strings.TrimSpace(flags.Manifest) == "" {
		return errors.New("--manifest is required")
	}
	if strings.TrimSpace(flags.Destination) == "" {
		return errors.New("--destination is required")
	}
	data, err := os.ReadFile(flags.Manifest)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest fbackup.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Version != fbackup.ManifestVersion {
		return fmt.Errorf("unsupported manifest version %q", manifest.Version)
	}
	return fbackup.Create(cmd.Context(), fbackup.CreateOptions{
		Destination: flags.Destination,
		Manifest:    manifest,
		Progress: func(id, status string) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", id, status)
		},
	})
}
