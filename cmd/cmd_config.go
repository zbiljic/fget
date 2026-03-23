package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:         "config",
	Short:       "Manage merged config",
	Annotations: map[string]string{"group": "config"},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

type configRuntimeContext struct {
	HomeDir       string
	Cwd           string
	XDGConfigHome string
}

func loadConfigRuntimeContext() (configRuntimeContext, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return configRuntimeContext{}, err
	}

	return configRuntimeContext{
		HomeDir:       homeDir,
		Cwd:           getWd(),
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}, nil
}

func expandHomePath(path, homeDir string) string {
	switch {
	case path == "~":
		return homeDir
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(homeDir, path[2:])
	default:
		return path
	}
}
