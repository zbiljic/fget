package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alitto/pond"
	"github.com/imdario/mergo"
	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tevino/abool/v2"

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

	// make a copy
	activeRepoPaths := art.New()
	repoPaths.ForEach(func(node art.Node) bool {
		activeRepoPaths.Insert(node.Key(), node.Value())
		return true
	})

	// fix
	startedAt := time.Now()

	i := 1
	it := repoPaths.Iterator()

	if config.Checkpoint != "" {
		i++

		// skip until previous checkpoint
		for ; it.HasNext(); i++ {
			node, _ := it.Next()
			repoPath := string(node.Key())

			activeRepoPaths.Delete(node.Key())

			if strings.EqualFold(repoPath, config.Checkpoint) {
				// found checkpoint
				break
			} else {
				// skip this path
				continue
			}
		}
	}

	// worker pool
	pool := pond.New(poolDefaultMaxWorkers, poolDefaultMaxCapacity)
	defer pool.StopAndWait()

	// task group associated to a context
	group, _ := pool.GroupContext(context.Background())

	for ; it.HasNext(); i++ {
		i := i

		node, _ := it.Next()
		repoPath := string(node.Key())

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
						pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...).Println(err.Error())
						return
					}

					pterm.Println()
					pterm.Printfln("[%d/%d] (active: %d)", i, repoPaths.Size(), activeRepoPaths.Size())
					pterm.Println(repoPath)
					pterm.NewStyle(pterm.ThemeDefault.InfoMessageStyle...).Println(project)
					pterm.NewStyle(pterm.ThemeDefault.ScopeStyle...).Println(branchName.Name().Short())
				})
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

				// find first still active path
				for it := activeRepoPaths.Iterator(); it.HasNext(); {
					node, _ := it.Next()
					repoPath := string(node.Key())

					config.Checkpoint = repoPath
					break
				}

				activeRepoPaths.Delete(art.Key(repoPath))

				if err := saveCheckpointConfigState(baseDir, configStateName, config, i); err != nil {
					pterm.NewStyle(pterm.ThemeDefault.ErrorMessageStyle...).Println(err.Error())
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
				return err
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	pterm.Success.WithPrefix(pterm.Prefix{
		Style: pterm.Success.Prefix.Style,
		Text:  configStateName,
	}).Println(fmt.Sprintf("took %s", time.Since(startedAt).Round(time.Millisecond).String()))

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
