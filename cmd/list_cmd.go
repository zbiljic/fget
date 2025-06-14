package cmd

import (
	"encoding/json"
	"io"

	"dario.cat/mergo"
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

// Output format constants
const (
	OutputFormatText = "text"
	OutputFormatJSON = "json"
)

var listCmdFlags = listOptions{
	OutputFormat: "",
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVarP(&listCmdFlags.OutputFormat, "output", "o", OutputFormatText, "Output format: text|json")
}

type listOptions struct {
	Roots        []string
	OutputFormat string
}

type repoInfo struct {
	Path    string `json:"path"`
	URL     string `json:"url"`
	Branch  string `json:"branch,omitempty"`
	IsClean bool   `json:"is_clean,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	opts, err := parseListArgs(args)
	if err != nil {
		return err
	}

	if err := mergo.Merge(&opts, listCmdFlags); err != nil {
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

	var repos []repoInfo

	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPath := string(node.Key())

		project, url, ref, err := gitProjectInfo(repoPath)
		if err != nil {
			return err
		}

		if opts.OutputFormat == OutputFormatJSON {
			branch := ""
			if ref != nil {
				branch = ref.Name().Short()
			}

			repos = append(repos, repoInfo{
				Path:   project,
				URL:    url,
				Branch: branch,
			})
		} else {
			pterm.Println(project)
		}
	}

	if opts.OutputFormat == OutputFormatJSON {
		return outputJSON(cmd.OutOrStdout(), repos)
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

func outputJSON(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
