package cmd

import (
	"context"
	"time"

	"github.com/alitto/pond"
	"github.com/imdario/mergo"
	art "github.com/plar/go-adaptive-radix-tree/v2"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var fixCmd = &cobra.Command{
	Use:         "fix",
	Short:       "Fixes inconsistencies found in the local repository",
	Annotations: map[string]string{"group": "update"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runFix,
}

var fixCmdFlags = &fixOptions{}

func init() {
	fixCmd.Flags().BoolVar(&fixCmdFlags.DryRun, "dry-run", false, "Displays the operations that would be performed using the specified command without actually running them")
	fixCmd.Flags().Uint16VarP(&fixCmdFlags.MaxWorkers, "workers", "j", poolDefaultMaxWorkers, "Set the maximum number of workers to use")
	fixCmd.Flags().BoolVarP(&fixCmdFlags.NoErrors, "no-errors", "s", false, "Suppress some errors")
	fixCmd.Flags().BoolVarP(&fixCmdFlags.OnlyUpdated, "only-updated", "u", false, "Print only updated projects")
	fixCmd.Flags().DurationVar(&fixCmdFlags.ExecTimeout, "exec-timeout", 0, "Duration after which process should stop")

	rootCmd.AddCommand(fixCmd)
}

type fixOptions struct {
	Roots       []string
	DryRun      bool
	MaxWorkers  uint16
	NoErrors    bool
	OnlyUpdated bool
	ExecTimeout time.Duration
}

func runFix(cmd *cobra.Command, args []string) error {
	cmdName := cmd.Name()
	runFn := gitRunFix

	opts, err := parseFixArgs(args)
	if err != nil {
		return err
	}

	if err := mergo.Merge(&opts, fixCmdFlags); err != nil {
		return err
	}

	// for configuration
	baseDir := opts.Roots[0]

	config, err := loadOrCreateConfigState(baseDir, cmdName, opts.Roots...)
	if err != nil {
		return err
	}

	defer func() {
		if err := finishConfigState(baseDir, cmdName, config); err != nil {
			ptermErrorMessageStyle.Println(err.Error())
		}
	}()

	if len(config.Paths) == 0 {
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

		config.TotalCount = repoPaths.Size()

		repoPaths.ForEach(func(node art.Node) bool {
			config.Paths = append(config.Paths, string(node.Key()))
			return true
		})

		if err := saveConfigState(baseDir, cmdName, config); err != nil {
			return err
		}

		spinner.Stop() //nolint:errcheck
	}

	// make a copy
	activeRepoPaths := make([]string, len(config.Paths))
	copy(activeRepoPaths, config.Paths)

	cleanupFn := func(repoPath string, index int, err error) error {
		if err != nil {
			if opts.NoErrors {
				return nil
			}
			return err
		}

		// update active
		activeRepoPaths = lo.Without(activeRepoPaths, repoPath)

		config.Paths = activeRepoPaths

		if err := saveCheckpointConfigState(baseDir, cmdName, config, index); err != nil {
			ptermErrorMessageStyle.Println(err.Error())
		}

		return nil
	}

	// start
	startedAt := time.Now()

	startOffset := 1 + config.TotalCount - len(activeRepoPaths)

	// worker pool
	pool := pond.New(int(opts.MaxWorkers), poolDefaultMaxCapacity)
	defer pool.StopAndWait()

	ctx := cmd.Context()

	if opts.ExecTimeout > 0 {
		var ctxCancelFn context.CancelFunc

		ctx, ctxCancelFn = context.WithTimeout(ctx, opts.ExecTimeout)
		defer ctxCancelFn()
	}

	// task group associated to a context
	group, ctx := pool.GroupContext(ctx)

	for i, path := range config.Paths {
		i := i + startOffset

		repoPath := path

		// context setup
		ctx = context.WithValue(ctx, ctxKeyDryRun{}, opts.DryRun)
		ctx = context.WithValue(ctx, ctxKeyOnlyUpdated{}, opts.OnlyUpdated)

		task := taskUpdateFn(
			ctx,
			cmdName,
			config,
			i,
			repoPath,
			runFn,
			cleanupFn,
		)

		group.Submit(task)
	}

	if err := group.Wait(); err != nil {
		return err
	}

	pterm.Println()
	ptermSuccessWithPrefixText(cmdName).
		Printfln("took %s (total: %s)",
			time.Since(startedAt).Round(time.Millisecond).String(),
			time.Since(config.CreateTime).Round(time.Millisecond).String(),
		)

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
