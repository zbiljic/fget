package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"dario.cat/mergo"
	"github.com/alitto/pond"
	art "github.com/plar/go-adaptive-radix-tree/v2"
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
	MaxWorkers:   poolDefaultMaxWorkers,
	SortBy:       "", // No sorting by default
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().VarP(
		enumflag.New(&listCmdFlags.OutputFormat, "output", OutputFormatIds, enumflag.EnumCaseInsensitive),
		"output", "o",
		"Output format: text|json|table")
	listCmd.Flags().Uint16VarP(&listCmdFlags.MaxWorkers, "workers", "j", poolDefaultMaxWorkers, "Set the maximum number of workers to use")
	listCmd.Flags().StringVarP(&listCmdFlags.SortBy, "sort", "s", "",
		"Sort repositories by: [±]time|[±]name|[±]commits (prefix with - for reverse order)")
}

type listOptions struct {
	Roots        []string
	OutputFormat OutputFormat
	MaxWorkers   uint16
	SortBy       string // Format: [±]field (e.g., "time", "-time", "name", etc.)
}

type repoInfo struct {
	Path        string    `json:"path"`
	URL         string    `json:"url"`
	Branch      string    `json:"branch,omitempty"`
	IsClean     bool      `json:"is_clean,omitempty"`
	LastUpdated time.Time `json:"last_updated,omitempty"`
	CommitCount int       `json:"commit_count,omitempty"`
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

	// For text output, stop spinner early since we print immediately
	if opts.OutputFormat == OutputFormatText {
		spinner.Stop() //nolint:errcheck
		return runListTextOutput(repoPaths)
	}

	// For other formats, we need to gather detailed info
	spinner.UpdateText("processing repositories...")

	return runListDetailedOutput(cmd.Context(), repoPaths, opts, func() {
		spinner.Stop() //nolint:errcheck
	})
}

// runListTextOutput prints the project names for each repository
func runListTextOutput(repoPaths art.Tree) error {
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

// runListDetailedOutput gathers detailed info for each repository
// and prints it in the specified format
func runListDetailedOutput(
	ctx context.Context,
	repoPaths art.Tree,
	opts listOptions,
	beforeOutput func(),
) error {
	// convert iterator to slice for parallel processing
	var repoPathSlice []string
	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPathSlice = append(repoPathSlice, string(node.Key()))
	}

	// worker pool
	pool := pond.New(int(opts.MaxWorkers), poolDefaultMaxCapacity)
	defer pool.StopAndWait()

	// task group associated to a context
	group, ctx := pool.GroupContext(ctx)

	// channel to collect results
	results := make(chan repoInfo, len(repoPathSlice))
	var resultsMutex sync.Mutex
	var repos []repoInfo

	for _, repoPath := range repoPathSlice {
		task := func() error {
			repo, err := processRepoInfo(repoPath)
			if err != nil {
				return err
			}

			results <- repo
			return nil
		}

		group.Submit(task)
	}

	// collect results as they complete
	go func() {
		for i := 0; i < len(repoPathSlice); i++ {
			select {
			case repo := <-results:
				resultsMutex.Lock()
				repos = append(repos, repo)
				resultsMutex.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := group.Wait(); err != nil {
		return err
	}

	// Sort repositories if requested
	if opts.SortBy != "" && len(repos) > 0 {
		reverse := false
		field := opts.SortBy

		// Check if reverse order is requested (prefixed with -)
		if strings.HasPrefix(field, "-") {
			reverse = true
			field = field[1:]
		} else if strings.HasPrefix(field, "+") {
			// Explicit ascending order (optional + prefix)
			field = field[1:]
		}

		// Sort based on the specified field
		sort.Slice(repos, func(i, j int) bool {
			var result bool

			switch field {
			case "time", "date", "updated":
				// Sort by last update time
				result = repos[i].LastUpdated.After(repos[j].LastUpdated)
			case "name", "path":
				// Sort by repository name/path
				result = repos[i].Path < repos[j].Path
			case "commits", "count":
				// Sort by commit count
				result = repos[i].CommitCount > repos[j].CommitCount
			default:
				// Default to sorting by path
				result = repos[i].Path < repos[j].Path
			}

			// Reverse the result if requested
			if reverse {
				return !result
			}
			return result
		})
	}

	// Call the beforeOutput callback after all processing is complete
	// but before printing output
	beforeOutput()

	// Output based on format
	switch opts.OutputFormat {
	case OutputFormatJSON:
		return outputJSON(pterm.DefaultBasicText.WithWriter(dynamicOutput).Writer, repos)
	case OutputFormatTable:
		return outputTable(pterm.DefaultBasicText.WithWriter(dynamicOutput).Writer, repos)
	}

	return nil
}

func processRepoInfo(repoPath string) (repoInfo, error) {
	project, url, ref, err := gitProjectInfo(repoPath)
	if err != nil {
		return repoInfo{}, err
	}

	branch := ""
	if ref != nil {
		branch = ref.Name().Short()
	}

	isClean, err := gitIsClean(context.Background(), repoPath)
	if err != nil {
		return repoInfo{}, err
	}

	lastUpdated, err := gitLastCommitDate(repoPath)
	if err != nil {
		return repoInfo{}, err
	}

	commitCount, err := gitRepoCommitCount(repoPath)
	if err != nil {
		return repoInfo{}, err
	}

	return repoInfo{
		Path:        project,
		URL:         url,
		Branch:      branch,
		IsClean:     isClean,
		LastUpdated: lastUpdated,
		CommitCount: commitCount,
	}, nil
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
		opts.Roots = append(opts.Roots, getWd())
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
	maxCommitCountWidth := len("Commits")

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
	maxCommitCountWidth += 2

	// Write markdown table header with padded columns
	headerRow := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s | %-*s |\n",
		maxPathWidth, "Repository",
		maxURLWidth, "URL",
		maxBranchWidth, "Branch",
		maxStatusWidth, "Status",
		maxLastUpdatedWidth, "Last Updated",
		maxCommitCountWidth, "Commits")
	if _, err := io.WriteString(w, headerRow); err != nil {
		return err
	}

	// Write separator row with uniform width
	separatorRow := fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
		strings.Repeat("-", maxPathWidth),
		strings.Repeat("-", maxURLWidth),
		strings.Repeat("-", maxBranchWidth),
		strings.Repeat("-", maxStatusWidth),
		strings.Repeat("-", maxLastUpdatedWidth),
		strings.Repeat("-", maxCommitCountWidth))
	if _, err := io.WriteString(w, separatorRow); err != nil {
		return err
	}

	// Write table rows with padded columns
	for _, repo := range repos {
		status := "clean"
		if !repo.IsClean {
			status = "modified"
		}

		row := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s | %-*d |\n",
			maxPathWidth, repo.Path,
			maxURLWidth, repo.URL,
			maxBranchWidth, repo.Branch,
			maxStatusWidth, status,
			maxLastUpdatedWidth, repo.LastUpdated.Format(lastUpdatedFormat),
			maxCommitCountWidth, repo.CommitCount)
		if _, err := io.WriteString(w, row); err != nil {
			return err
		}
	}

	return nil
}
