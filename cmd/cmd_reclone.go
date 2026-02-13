package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var recloneCmd = &cobra.Command{
	Use:         "reclone",
	Aliases:     []string{"reset"},
	Short:       "Delete and clone repositories again from their remote URL",
	Annotations: map[string]string{"group": "update"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runReclone,
}

var recloneCmdFlags = &recloneOptions{}

func init() {
	recloneCmd.Flags().BoolVar(&recloneCmdFlags.DryRun, "dry-run", false, "Displays the operations that would be performed using the specified command without actually running them")
	recloneCmd.Flags().BoolVarP(&recloneCmdFlags.AssumeYes, "yes", "y", false, "Skip confirmation prompt")

	rootCmd.AddCommand(recloneCmd)
}

type recloneOptions struct {
	RepoPaths []string
	DryRun    bool
	AssumeYes bool
}

func runReclone(cmd *cobra.Command, args []string) error {
	opts, err := parseRecloneArgs(args)
	if err != nil {
		return err
	}

	opts.DryRun = recloneCmdFlags.DryRun
	opts.AssumeYes = recloneCmdFlags.AssumeYes

	if opts.DryRun {
		opts.AssumeYes = true
	}

	if !opts.DryRun {
		if err := ensureCwdOutsideTargets(opts.RepoPaths); err != nil {
			return err
		}
	}

	if err := confirmReclone(opts); err != nil {
		return err
	}

	startedAt := time.Now()

	for i, repoPath := range opts.RepoPaths {
		index := i + 1

		printProjectInfoHeaderFn := func() {
			_, remoteURL, branchName, err := gitProjectInfo(repoPath)
			if err != nil {
				pterm.Println()
				pterm.Printfln("[%d/%d]", index, len(opts.RepoPaths))
				pterm.Println(repoPath)
				ptermErrorMessageStyle.Println(err.Error())
				return
			}

			pterm.Println()
			pterm.Printfln("[%d/%d]", index, len(opts.RepoPaths))
			pterm.Println(repoPath)
			ptermInfoMessageStyle.Println(remoteURL)
			ptermScopeStyle.Println(branchName.Name().Short())
		}

		taskCtx := context.WithValue(cmd.Context(), ctxKeyPrintProjectInfoHeaderFn{}, printProjectInfoHeaderFn)
		taskCtx = context.WithValue(taskCtx, ctxKeyDryRun{}, opts.DryRun)

		if err := gitRunReclone(taskCtx, repoPath); err != nil {
			ptermErrorMessageStyle.Printfln("reclone '%s': %s", repoPath, err.Error())
			return err
		}
	}

	pterm.Println()
	ptermSuccessWithPrefixText(cmd.Name()).
		Printfln("took %s", time.Since(startedAt).Round(time.Millisecond).String())

	return nil
}

func parseRecloneArgs(args []string) (recloneOptions, error) {
	opts := recloneOptions{}

	if len(args) == 0 {
		return opts, errors.New("requires at least 1 local repository path argument")
	}

	for _, arg := range args {
		path, err := fsfind.DirAbsPath(arg)
		if err != nil {
			return opts, err
		}

		opts.RepoPaths = append(opts.RepoPaths, path)
	}

	return opts, nil
}

func confirmReclone(opts recloneOptions) error {
	if opts.AssumeYes {
		return nil
	}

	if isNotTerminal {
		return errors.New("reclone requires confirmation; rerun with --yes for non-interactive use")
	}

	confirmMsg := fmt.Sprintf(
		"Delete and re-clone %d repository path(s): %s",
		len(opts.RepoPaths),
		strings.Join(opts.RepoPaths, ", "),
	)

	ok, err := pterm.DefaultInteractiveConfirm.
		WithDefaultValue(false).
		Show(confirmMsg)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("operation canceled")
	}

	return nil
}

func ensureCwdOutsideTargets(repoPaths []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	for _, repoPath := range repoPaths {
		inside, err := isPathWithin(repoPath, cwd)
		if err != nil {
			return err
		}

		if inside {
			return fmt.Errorf("current working directory '%s' is inside target repository '%s'; run reclone from outside the target repository", cwd, repoPath)
		}
	}

	return nil
}

func isPathWithin(basePath, targetPath string) (bool, error) {
	baseAbsPath, err := normalizePath(basePath)
	if err != nil {
		return false, err
	}

	targetAbsPath, err := normalizePath(targetPath)
	if err != nil {
		return false, err
	}

	relPath, err := filepath.Rel(baseAbsPath, targetAbsPath)
	if err != nil {
		return false, err
	}

	if relPath == "." {
		return true, nil
	}

	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return false, nil
	}

	return true, nil
}

func normalizePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	normalizedPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return normalizedPath, nil
	}

	// Path might not exist; absolute path is still useful for containment checks.
	return absPath, nil
}
