package cmd

import (
	"context"
	"strings"

	"github.com/imdario/mergo"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var fixCmd = &cobra.Command{
	Use:         "fix",
	Short:       "List local repositories",
	Annotations: map[string]string{"group": "update"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runFix,
}

var fixCmdFlags = &fixOptions{}

func init() {
	fixCmd.Flags().BoolVar(&fixCmdFlags.DryRun, "dry-run", false, "Displays the operations that would be performed using the specified command without actually running them")

	rootCmd.AddCommand(fixCmd)
}

type fixOptions struct {
	Roots  []string
	DryRun bool
}

func runFix(cmd *cobra.Command, args []string) error {
	opts, err := parseFixArgs(args)
	if err != nil {
		return err
	}

	if err := mergo.Merge(&opts, fixCmdFlags); err != nil {
		return err
	}

	// for configuration
	baseDir := opts.Roots[0]
	configStateName := cmd.Name()

	config, err := loadOrCreateConfigState(baseDir, configStateName, opts.Roots...)
	if err != nil {
		return err
	}

	defer func() {
		if err := finishConfigState(baseDir, configStateName, config); err != nil {
			pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...).Println(err.Error())
		}
	}()

	spinner, err := pterm.DefaultSpinner.
		WithWriter(dynamicOutput).
		WithRemoveWhenDone(true).
		Start("finding repositories...")
	if err != nil {
		return err
	}

	repoPaths, err := fsfind.GitDirectoriesTree(opts.Roots...)
	if err != nil {
		return err
	}

	spinner.Stop() //nolint:errcheck

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKeyDryRun{}, opts.DryRun)

	it := repoPaths.Iterator()

	if config.Checkpoint != "" {
		// skip until previous checkpoint
		for it.HasNext() {
			node, _ := it.Next()
			repoPath := string(node.Key())

			if strings.EqualFold(repoPath, config.Checkpoint) {
				// found checkpoint
				break
			} else {
				// skip this path
				continue
			}
		}
	}

	for i := 0; it.HasNext(); i++ {
		node, _ := it.Next()
		repoPath := string(node.Key())

		if err := saveCheckpointConfigState(baseDir, configStateName, config, i); err != nil {
			return err
		}

		config.Checkpoint = repoPath

		project, branchName, err := gitProjectInfo(repoPath)
		if err != nil {
			return err
		}

		pterm.Println(repoPath)
		pterm.NewStyle(pterm.ThemeDefault.InfoMessageStyle...).Println(project)
		pterm.NewStyle(pterm.ThemeDefault.ScopeStyle...).Println(branchName.Name().Short())

		if err := gitFixReferences(ctx, repoPath); err != nil {
			return err
		}

		if err := gitMakeClean(ctx, repoPath); err != nil {
			return err
		}

		if err := gitUpdateDefaultBranch(ctx, repoPath); err != nil {
			return err
		}

		pterm.Println()
	}

	// in order to clear configuration file
	config.Checkpoint = ""

	return nil
}

func parseFixArgs(args []string) (fixOptions, error) {
	opts := fixOptions{}

	if len(args) > 0 {
		for _, arg := range args {
			path, err := fsfind.DirAbsPath(arg)
			if err != nil {
				return opts, err
			}

			opts.Roots = append(opts.Roots, path)
		}
	} else {
		// fallback to current working directory
		opts.Roots = append(opts.Roots, GetWd())
	}

	return opts, nil
}
