package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alitto/pond"
	"github.com/imdario/mergo"
	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/tevino/abool/v2"

	"github.com/zbiljic/fget/pkg/fsfind"
)

const fixCmdName = "fix"

var fixCmd = &cobra.Command{
	Use:         fixCmdName,
	Short:       "Fixes inconsistencies found in the local repository",
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

	config, err := loadOrCreateConfigState(baseDir, fixCmdName, opts.Roots...)
	if err != nil {
		return err
	}

	defer func() {
		if err := finishConfigState(baseDir, fixCmdName, config); err != nil {
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

		if err := saveConfigState(baseDir, fixCmdName, config); err != nil {
			return err
		}

		spinner.Stop() //nolint:errcheck
	}

	// make a copy
	activeRepoPaths := make([]string, len(config.Paths))
	copy(activeRepoPaths, config.Paths)

	// start
	startedAt := time.Now()

	startOffset := 1 + config.TotalCount - len(activeRepoPaths)

	// worker pool
	pool := pond.New(poolDefaultMaxWorkers, poolDefaultMaxCapacity)
	defer pool.StopAndWait()

	// task group associated to a context
	group, _ := pool.GroupContext(context.Background())

	for i, path := range config.Paths {
		i := i + startOffset

		repoPath := path

		group.Submit(func() error {
			var (
				printProjectInfoHeaderOnce sync.Once
				isUpdateMutexLocked        = abool.New()
				errored                    = abool.New()
			)

			printProjectInfoHeaderFn := func() {
				printProjectInfoHeaderOnce.Do(func() {
					project, branchName, err := gitProjectInfo(repoPath)
					if err != nil {
						ptermErrorMessageStyle.Println(err.Error())
						return
					}

					pterm.Println()
					pterm.Printfln("[%d/%d] (active: %d)", i, config.TotalCount, len(activeRepoPaths))
					pterm.Println(repoPath)
					ptermInfoMessageStyle.Println(project)
					ptermScopeStyle.Println(branchName.Name().Short())
				})
			}

			printErrorFn := func(err error) {
				printProjectInfoHeaderFn()
				ptermErrorMessageStyle.Println(err.Error())
			}

			// cleanup
			defer func() {
				if isUpdateMutexLocked.IsNotSet() {
					updateMutex.Lock()
				}
				defer updateMutex.Unlock()

				printProjectInfoHeaderFn()

				if errored.IsSet() {
					return
				}

				// update active
				activeRepoPaths = lo.Without(activeRepoPaths, repoPath)

				config.Paths = activeRepoPaths

				if err := saveCheckpointConfigState(baseDir, fixCmdName, config, i); err != nil {
					ptermErrorMessageStyle.Println(err.Error())
				}
			}()

			// context setup
			ctx := context.Background()
			ctx = context.WithValue(ctx, ctxKeyDryRun{}, opts.DryRun)
			ctx = context.WithValue(ctx, ctxKeyPrintProjectInfoHeaderFn{}, printProjectInfoHeaderFn)
			ctx = context.WithValue(ctx, ctxKeyIsUpdateMutexLocked{}, isUpdateMutexLocked)
			ctx = context.WithValue(ctx, ctxKeyShouldUpdateMutexUnlock{}, false)

			if err := gitRunFix(ctx, repoPath); err != nil {
				errored.Set()
				printErrorFn(err)
				return fmt.Errorf("%s '%s': %w", fixCmdName, repoPath, err)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	pterm.Println()
	ptermSuccessWithPrefixText(fixCmdName).
		Printfln("took %s (total: %s)",
			time.Since(startedAt).Round(time.Millisecond).String(),
			time.Since(config.CreateTime).Round(time.Millisecond).String(),
		)

	// in order to clear configuration file
	config.Paths = nil

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
