package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

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

	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPath := string(node.Key())

		project, _, _, err := gitProjectInfo(repoPath)
		if err != nil {
			return err
		}

		pterm.Println(project)
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
