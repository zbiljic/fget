package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/alitto/pond/v2"
	"github.com/go-git/go-git/v5"
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
	StateFilter:  stateFilterAll,
	ShowState:    false,
	StateTimeout: 10 * time.Second,
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
	listCmd.Flags().BoolVarP(&listCmdFlags.ShowState, "show-state", "A", false,
		"Include remote state in the output (active/inactive)")
	listCmd.Flags().StringVarP(&listCmdFlags.StateFilter, "state", "a", stateFilterAll,
		"Filter repositories by remote state: all|active|inactive (alias: archived)")
}

type listOptions struct {
	Roots        []string
	OutputFormat OutputFormat
	MaxWorkers   uint16
	SortBy       string        // Format: [±]field (e.g., "time", "-time", "name", etc.)
	ShowState    bool          // Include state in output
	StateFilter  string        // all|active|inactive
	StateTimeout time.Duration // Timeout for one remote state check
}

type repoInfo struct {
	Path        string    `json:"path"`
	URL         string    `json:"url"`
	Branch      string    `json:"branch,omitempty"`
	IsClean     bool      `json:"is_clean,omitempty"`
	LastUpdated time.Time `json:"last_updated,omitempty"`
	CommitCount int       `json:"commit_count,omitempty"`
	Active      *bool     `json:"active,omitempty"`
}

const (
	stateFilterAll      = "all"
	stateFilterActive   = "active"
	stateFilterInactive = "inactive"
	repoStateActive     = "active"
	repoStateInactive   = "inactive"
)

func runList(cmd *cobra.Command, args []string) error {
	opts, err := parseListArgs(args)
	if err != nil {
		return err
	}

	if err := mergo.Merge(&opts, listCmdFlags); err != nil {
		return err
	}

	normalizedStateFilter, err := normalizeStateFilter(opts.StateFilter)
	if err != nil {
		return err
	}
	opts.StateFilter = normalizedStateFilter
	if opts.StateFilter != stateFilterAll {
		// when filtering by state, always print it so the result is explicit.
		opts.ShowState = true
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
		return runListTextOutput(cmd.Context(), repoPaths, opts)
	}

	// For other formats, we need to gather detailed info
	spinner.UpdateText("processing repositories...")

	return runListDetailedOutput(cmd.Context(), repoPaths, opts, func() {
		spinner.Stop() //nolint:errcheck
	})
}

// runListTextOutput prints the project names for each repository
func runListTextOutput(ctx context.Context, repoPaths art.Tree, opts listOptions) error {
	for it := repoPaths.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		repoPath := string(node.Key())

		project, _, _, err := gitProjectInfo(repoPath)
		if err != nil {
			if isListSkippableRepoError(err) {
				continue
			}

			return err
		}

		active, checked := listRepoState(ctx, repoPath, opts)
		if checked && !shouldIncludeByState(opts.StateFilter, active) {
			continue
		}

		if opts.ShowState && checked {
			pterm.Printf("%s\t%s\n", project, formatRepoState(active))
			continue
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

	resultPool := pond.NewResultPool[*repoInfo](
		int(opts.MaxWorkers),
		pond.WithQueueSize(poolDefaultMaxCapacity),
	)
	defer resultPool.StopAndWait()

	group := resultPool.NewGroupContext(ctx)

	for _, repoPath := range repoPathSlice {
		group.SubmitErr(func() (*repoInfo, error) {
			repo, err := processRepoInfo(ctx, repoPath, opts)
			if err != nil {
				if isListSkippableRepoError(err) {
					return nil, nil
				}

				return nil, err
			}

			return repo, nil
		})
	}

	repoResults, err := group.Wait()
	if err != nil {
		return err
	}

	repos := make([]repoInfo, 0, len(repoResults))
	for _, repo := range repoResults {
		if repo != nil {
			repos = append(repos, *repo)
		}
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
		return outputTable(pterm.DefaultBasicText.WithWriter(dynamicOutput).Writer, repos, opts.ShowState)
	}

	return nil
}

func processRepoInfo(ctx context.Context, repoPath string, opts listOptions) (*repoInfo, error) {
	project, url, ref, err := gitProjectInfo(repoPath)
	if err != nil {
		return nil, err
	}

	branch := ""
	if ref != nil {
		branch = ref.Name().Short()
	}

	active, checked := listRepoState(ctx, repoPath, opts)
	if checked && !shouldIncludeByState(opts.StateFilter, active) {
		return nil, nil
	}

	isClean, err := gitIsClean(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	lastUpdated, err := gitLastCommitDate(repoPath)
	if err != nil {
		return nil, err
	}

	commitCount, err := gitRepoCommitCount(repoPath)
	if err != nil {
		return nil, err
	}

	repo := &repoInfo{
		Path:        project,
		URL:         url,
		Branch:      branch,
		IsClean:     isClean,
		LastUpdated: lastUpdated,
		CommitCount: commitCount,
	}
	if checked {
		repo.Active = &active
	}

	return repo, nil
}

func isListSkippableRepoError(err error) bool {
	return errors.Is(err, git.ErrRemoteNotFound)
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

func normalizeStateFilter(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", stateFilterAll:
		return stateFilterAll, nil
	case "active":
		return stateFilterActive, nil
	case "inactive", "archived":
		return stateFilterInactive, nil
	default:
		return "", fmt.Errorf("invalid --state value %q (allowed: all|active|inactive)", value)
	}
}

func shouldCheckState(opts listOptions) bool {
	return opts.ShowState || opts.StateFilter != stateFilterAll
}

func listRepoState(ctx context.Context, repoPath string, opts listOptions) (bool, bool) {
	if !shouldCheckState(opts) {
		return false, false
	}

	checkCtx := ctx
	if opts.StateTimeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, opts.StateTimeout)
		defer cancel()
	}

	_, err := gitFindRemoteHeadReference(checkCtx, repoPath)
	if err == nil || errors.Is(err, ErrGitMissingRemoteHeadReference) {
		return true, true
	}

	// for list output, any check error means inactive right now.
	return false, true
}

func shouldIncludeByState(filter string, active bool) bool {
	switch filter {
	case stateFilterActive:
		return active
	case stateFilterInactive:
		return !active
	default:
		return true
	}
}

func formatRepoState(active bool) string {
	if active {
		return repoStateActive
	}

	return repoStateInactive
}

func outputJSON(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func outputTable(w io.Writer, repos []repoInfo, showState bool) error {
	if len(repos) == 0 {
		return nil
	}

	// Find maximum width for each column
	maxPathWidth := len("Repository")
	maxURLWidth := len("URL")
	maxBranchWidth := len("Branch")
	maxStatusWidth := len("Status")
	maxStateWidth := len("State")
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
		if showState && repo.Active != nil {
			stateLabel := formatRepoState(*repo.Active)
			if len(stateLabel) > maxStateWidth {
				maxStateWidth = len(stateLabel)
			}
		}
	}

	// Add some padding
	maxPathWidth += 2
	maxURLWidth += 2
	maxBranchWidth += 2
	maxStatusWidth += 2
	maxStateWidth += 2
	maxLastUpdatedWidth += 2
	maxCommitCountWidth += 2

	if showState {
		// write markdown table header with padded columns
		headerRow := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s | %-*s | %-*s |\n",
			maxPathWidth, "Repository",
			maxURLWidth, "URL",
			maxBranchWidth, "Branch",
			maxStatusWidth, "Status",
			maxStateWidth, "State",
			maxLastUpdatedWidth, "Last Updated",
			maxCommitCountWidth, "Commits")
		if _, err := io.WriteString(w, headerRow); err != nil {
			return err
		}

		// write separator row with uniform width
		separatorRow := fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
			strings.Repeat("-", maxPathWidth),
			strings.Repeat("-", maxURLWidth),
			strings.Repeat("-", maxBranchWidth),
			strings.Repeat("-", maxStatusWidth),
			strings.Repeat("-", maxStateWidth),
			strings.Repeat("-", maxLastUpdatedWidth),
			strings.Repeat("-", maxCommitCountWidth))
		if _, err := io.WriteString(w, separatorRow); err != nil {
			return err
		}
	} else {
		// write markdown table header with padded columns
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

		// write separator row with uniform width
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
	}

	// Write table rows with padded columns
	for _, repo := range repos {
		status := "clean"
		if !repo.IsClean {
			status = "modified"
		}

		if showState {
			stateText := ""
			if repo.Active != nil {
				stateText = formatRepoState(*repo.Active)
			}

			row := fmt.Sprintf("| %-*s | %-*s | %-*s | %-*s | %-*s | %-*s | %-*d |\n",
				maxPathWidth, repo.Path,
				maxURLWidth, repo.URL,
				maxBranchWidth, repo.Branch,
				maxStatusWidth, status,
				maxStateWidth, stateText,
				maxLastUpdatedWidth, repo.LastUpdated.Format(lastUpdatedFormat),
				maxCommitCountWidth, repo.CommitCount)
			if _, err := io.WriteString(w, row); err != nil {
				return err
			}
			continue
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
