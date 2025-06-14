package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag/v2"

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

// OutputFormat represents the output format type
type OutputFormat enumflag.Flag

// Output format constants
const (
	OutputFormatText OutputFormat = iota
	OutputFormatJSON
	OutputFormatTable
)

// OutputFormatIds maps the enum values to their string representations
var OutputFormatIds = map[OutputFormat][]string{
	OutputFormatText:  {"text"},
	OutputFormatJSON:  {"json"},
	OutputFormatTable: {"table"},
}

var listCmdFlags = listOptions{
	OutputFormat: OutputFormatText,
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().VarP(
		enumflag.New(&listCmdFlags.OutputFormat, "output", OutputFormatIds, enumflag.EnumCaseInsensitive),
		"output", "o",
		"Output format: text|json|table")
}

type listOptions struct {
	Roots        []string
	OutputFormat OutputFormat
}

type repoInfo struct {
	Path        string    `json:"path"`
	URL         string    `json:"url"`
	Branch      string    `json:"branch,omitempty"`
	IsClean     bool      `json:"is_clean,omitempty"`
	LastUpdated time.Time `json:"last_updated,omitempty"`
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

		switch opts.OutputFormat {
		case OutputFormatJSON, OutputFormatTable:
			branch := ""
			if ref != nil {
				branch = ref.Name().Short()
			}

			isClean, err := gitIsClean(cmd.Context(), repoPath)
			if err != nil {
				return err
			}

			lastUpdated, err := gitLastCommitDate(repoPath)
			if err != nil {
				return err
			}

			repos = append(repos, repoInfo{
				Path:        project,
				URL:         url,
				Branch:      branch,
				IsClean:     isClean,
				LastUpdated: lastUpdated,
			})
		default:
			pterm.Println(project)
		}
	}

	switch opts.OutputFormat {
	case OutputFormatJSON:
		return outputJSON(cmd.OutOrStdout(), repos)
	case OutputFormatTable:
		return outputTable(cmd.OutOrStdout(), repos)
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

func outputTable(w io.Writer, repos []repoInfo) error {
	if len(repos) == 0 {
		return nil
	}

	// Find maximum width for each column
	maxPathWidth := len("Repository")
	maxURLWidth := len("URL")
	maxBranchWidth := len("Branch")
	maxStatusWidth := len("Status")
	maxLastUpdatedWidth := len("Last Updated")

	lastUpdatedFormat := time.DateOnly
	if len(lastUpdatedFormat) > maxLastUpdatedWidth {
		maxLastUpdatedWidth = len(lastUpdatedFormat)
	}

	for _, repo := range repos {
		if len(repo.Path) > maxPathWidth {
			maxPathWidth = len(repo.Path)
		}
		if len(repo.URL) > maxURLWidth {
			maxURLWidth = len(repo.URL)
		}
		if len(repo.Branch) > maxBranchWidth {
			maxBranchWidth = len(repo.Branch)
		}
	}

	// Add some padding
	maxPathWidth += 2
	maxURLWidth += 2
	maxBranchWidth += 2
	maxStatusWidth += 2
	maxLastUpdatedWidth += 2

	// Write markdown table header with padded columns
	headerRow := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s |\n",
		maxPathWidth, "Repository",
		maxURLWidth, "URL",
		maxBranchWidth, "Branch",
		maxStatusWidth, "Status",
		maxLastUpdatedWidth, "Last Updated")
	if _, err := io.WriteString(w, headerRow); err != nil {
		return err
	}

	// Write separator row with uniform width
	separatorRow := fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
		strings.Repeat("-", maxPathWidth),
		strings.Repeat("-", maxURLWidth),
		strings.Repeat("-", maxBranchWidth),
		strings.Repeat("-", maxStatusWidth),
		strings.Repeat("-", maxLastUpdatedWidth))
	if _, err := io.WriteString(w, separatorRow); err != nil {
		return err
	}

	// Write table rows with padded columns
	for _, repo := range repos {
		status := "clean"
		if !repo.IsClean {
			status = "modified"
		}

		row := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s |\n",
			maxPathWidth, repo.Path,
			maxURLWidth, repo.URL,
			maxBranchWidth, repo.Branch,
			maxStatusWidth, status,
			maxLastUpdatedWidth, repo.LastUpdated.Format(lastUpdatedFormat))
		if _, err := io.WriteString(w, row); err != nil {
			return err
		}
	}

	return nil
}
