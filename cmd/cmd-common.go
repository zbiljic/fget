package cmd

import (
	"context"
	"fmt"
	"sync"

	"github.com/pterm/pterm"
	"github.com/tevino/abool/v2"
)

func printProjectInfoContext(ctx context.Context) {
	if printProjectInfoHeaderFn, ok := ctx.Value(ctxKeyPrintProjectInfoHeaderFn{}).(func()); ok {
		printProjectInfoHeaderFn()
	}
}

func taskUpdateFn(
	ctx context.Context,
	cmdName string,
	config *configStateV2,
	index int,
	repoPath string,
	runFn func(context.Context, string) error,
	cleanupFn func(string, int, error) error,
) func() error {
	return func() error {
		var (
			printProjectInfoHeaderOnce sync.Once
			isUpdateMutexLocked        = abool.New()
		)

		printProjectInfoHeaderFn := func() {
			printProjectInfoHeaderOnce.Do(func() {
				project, branchName, err := gitProjectInfo(repoPath)
				if err != nil {
					ptermErrorMessageStyle.Println(err.Error())
					return
				}

				pterm.Println()
				pterm.Printfln("[%d/%d] (active: %d)", index, config.TotalCount, len(config.Paths))
				pterm.Println(repoPath)
				ptermInfoMessageStyle.Println(project)
				ptermScopeStyle.Println(branchName.Name().Short())
			})
		}

		// context setup
		ctx = context.WithValue(ctx, ctxKeyPrintProjectInfoHeaderFn{}, printProjectInfoHeaderFn)
		ctx = context.WithValue(ctx, ctxKeyIsUpdateMutexLocked{}, isUpdateMutexLocked)
		ctx = context.WithValue(ctx, ctxKeyShouldUpdateMutexUnlock{}, false)

		err := runFn(ctx, repoPath)
		// NOTE: error check comes after lock

		if isUpdateMutexLocked.IsNotSet() {
			updateMutex.Lock()
		}
		defer updateMutex.Unlock()

		printProjectInfoHeaderFn()

		// NOTE: delayed error check
		if err != nil {
			ptermErrorMessageStyle.Println(err.Error())
			err = fmt.Errorf("%s '%s': %w", cmdName, repoPath, err)
		}

		return cleanupFn(repoPath, index, err)
	}
}
