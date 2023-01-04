package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	giturls "github.com/whilp/git-urls"

	"github.com/zbiljic/fget/pkg/fsfind"
)

var listCmd = &cobra.Command{
	Use:         "list",
	Aliases:     []string{"ls"},
	Short:       "List local repositories",
	Annotations: map[string]string{"group": "view"},
	Args:        cobra.ArbitraryArgs,
	RunE:        runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

type listOptions struct {
	Roots []string
}

func runList(cmd *cobra.Command, args []string) error {
	opts, err := parseListArgs(args)
	if err != nil {
		return err
	}

	repoPaths, err := fsfind.GitDirectoriesTree(opts.Roots...)
	if err != nil {
		return err
	}

	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPath := string(node.Key())

		repo, err := git.PlainOpen(repoPath)
		if err != nil {
			return err
		}

		remote, err := repo.Remote(git.DefaultRemoteName)
		if err != nil {
			return err
		}

		if repoURL := remote.Config().URLs[0]; repoURL != "" {
			// parse URI
			parsedURI, err := giturls.Parse(repoURL)
			if err != nil {
				return err
			}

			project := filepath.Join(parsedURI.Host, parsedURI.Path)

			fmt.Fprintln(os.Stdout, project)
		}
	}

	return nil
}

func parseListArgs(args []string) (listOptions, error) {
	opts := listOptions{}

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
