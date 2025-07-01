package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/version"
)

var versionCmd = &cobra.Command{
	Use:         "version",
	Short:       "Print version information",
	Annotations: map[string]string{"group": "other"},
	Args:        cobra.NoArgs,
	RunE:        printVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func printVersion(cmd *cobra.Command, args []string) error {
	_, err := fmt.Fprintf(os.Stdout, "%s\n", version.Version)
	return err
}
