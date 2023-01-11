package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var cloneCmd = &cobra.Command{
	Use:         "clone",
	Short:       "Clone a repository into a new directory",
	Annotations: map[string]string{"group": "update"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runClone,
}

func init() {
	rootCmd.AddCommand(cloneCmd)
}

type cloneOptions struct {
	// The remote repositories URLs to clone from.
	URLs []*url.URL
	// The name of the directory root to clone into.
	RootDirectory string
}

func runClone(cmd *cobra.Command, args []string) error {
	opts, err := parseCloneArgs(args)
	if err != nil {
		return err
	}

	for _, url := range opts.URLs {
		var (
			domainDirectory = filepath.Join(opts.RootDirectory, url.Host)
			projectID       = filepath.Join(url.Host, url.Path)
			repoURL         = url.String()
		)

		// check domain directory
		domainDirFileInfo, err := os.Stat(domainDirectory)
		if err != nil {
			return fmt.Errorf("domain path: %v", err)
		}

		if !domainDirFileInfo.IsDir() {
			return fmt.Errorf("not directory: %s", domainDirectory)
		}

		// create path into which to clone
		repoPath := filepath.Join(opts.RootDirectory, projectID)

		pterm.Println(repoPath)
		pterm.Println(projectID)

		// clone
		startedAt := time.Now()

		spinner, err := pterm.DefaultSpinner.
			WithWriter(dynamicOutput).
			Start("cloning...")
		if err != nil {
			return err
		}

		buf := bytes.NewBuffer(nil)

		_, err = git.PlainClone(repoPath, false, &git.CloneOptions{
			URL:      repoURL,
			Progress: buf,
		})
		if err != nil {
			if errors.Is(err, git.ErrRepositoryAlreadyExists) {
				spinner.Warning(err.Error())
				pterm.Println()
				continue
			}

			spinner.Fail(err.Error())
			pterm.Println()
			return err
		}

		if buf.Len() > 0 {
			pterm.Fprintln(dynamicOutput)
			pterm.Fprintln(dynamicOutput, buf.String())
		}

		spinner.Success(fmt.Sprintf("took %s", time.Since(startedAt).Round(time.Millisecond).String()))
		pterm.Println()
	}

	return nil
}

func parseCloneArgs(args []string) (cloneOptions, error) {
	opts := cloneOptions{}

	urlArgsLastIndex := 0

	for i, arg := range args {
		// parse URI
		parsedURI, err := url.ParseRequestURI(arg)
		if err != nil {
			// last argument might be path
			if i == len(args)-1 {
				break
			}
			return opts, err
		}

		if parsedURI.Host == "" {
			continue
		}

		opts.URLs = append(opts.URLs, parsedURI)
		urlArgsLastIndex = i
	}

	if len(opts.URLs) == 0 {
		return opts, errors.New("requires at least 1 URL argument, received 0")
	}

	if len(args) > urlArgsLastIndex+1 {
		lastArg := args[len(args)-1]

		path, err := fsfind.DirAbsPath(lastArg)
		if err != nil {
			return opts, err
		}

		opts.RootDirectory = path
	}

	if opts.RootDirectory == "" {
		// fallback to current working directory
		opts.RootDirectory = GetWd()
	}

	return opts, nil
}

func isCloneCmd(cmd *cobra.Command, args []string) bool {
	if len(args) < 1 {
		return false
	}

	if _, err := parseCloneArgs(args); err != nil {
		return false
	}

	return true
}
