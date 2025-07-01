package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/version"
)

// AppName - the name of the application.
const AppName = "fget"

var rootCmd = &cobra.Command{
	Use: AppName,
	Version: versionString(
		VersionInfo{
			Version: version.Version,
			Commit:  version.GitCommit,
			BuiltBy: version.BuiltBy,
		},
	),
	Short:         "Remote repositories manager.",
	Long:          `Remote repositories manager.`,
	Args:          cobra.ArbitraryArgs,
	RunE:          runRoot,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
		cmd.SetContext(ctx)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called my main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if cmd, err := rootCmd.ExecuteC(); err != nil {
		if strings.Contains(err.Error(), "arg(s)") || strings.Contains(err.Error(), "usage") {
			cmd.Usage() //nolint:errcheck
		}

		ExitIfErr(os.Stderr, err)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	switch {
	case isCloneCmd(cmd, args):
		return runClone(cmd, args)
	default:
		cmd.Usage() //nolint:errcheck
		return errors.New("unknown command arguments")
	}
}
