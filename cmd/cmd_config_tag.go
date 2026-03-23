package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/fsfind"
)

var configTagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Manage repository tags",
}

var configTagAddCmd = &cobra.Command{
	Use:   "add [repo] <tag...>",
	Short: "Add one or more tags to a repository",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigTagAdd,
}

var configTagRemoveCmd = &cobra.Command{
	Use:   "remove [repo] <tag...>",
	Short: "Remove one or more tags from a repository",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigTagRemove,
}

var configTagListCmd = &cobra.Command{
	Use:   "list [repo]",
	Short: "List tags",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigTagList,
}

type (
	configTagGitRootFn     func(path string) (string, error)
	configTagFindReposFn   func(ctx context.Context, roots ...string) ([]string, error)
	configTagInspectRepoFn func(path string) (fconfig.RepoMetadata, error)
)

type configTagModifyRequest struct {
	RepoSelectors        []string
	Tags                 []string
	RequiresConfirmation bool
}

type configTagOptions struct {
	AssumeYes bool
}

var configTagCmdFlags = configTagOptions{}

func init() {
	configTagCmd.PersistentFlags().BoolVarP(
		&configTagCmdFlags.AssumeYes,
		"yes",
		"y",
		false,
		"Skip confirmation prompt when applying tags to discovered repositories",
	)

	rootCmd.AddCommand(configTagCmd)
	configTagCmd.AddCommand(configTagAddCmd)
	configTagCmd.AddCommand(configTagRemoveCmd)
	configTagCmd.AddCommand(configTagListCmd)
}

func runConfigTagAdd(cmd *cobra.Command, args []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	catalog, catalogPath, err := loadCatalogForRuntimeContext(runtimeCtx)
	if err != nil {
		return err
	}

	req, err := resolveConfigTagModifyRequest(
		cmd.Context(),
		args,
		runtimeCtx.Cwd,
		catalog,
		fsfind.GitRootPath,
		findGitRepoPaths,
		inspectRepoMetadata,
	)
	if err != nil {
		return err
	}

	if err := confirmConfigTagModify("add", req); err != nil {
		return err
	}

	for _, repoSelector := range req.RepoSelectors {
		if err := fconfig.AddTags(catalog, repoSelector, req.Tags); err != nil {
			return err
		}
	}

	if err := fconfig.SaveCatalog(catalogPath, catalog); err != nil {
		return err
	}

	if len(req.RepoSelectors) == 1 {
		ptermSuccessMessageStyle.Printfln("tags updated for %s", req.RepoSelectors[0])
		return nil
	}

	ptermSuccessMessageStyle.Printfln("tags updated for %d repositories", len(req.RepoSelectors))
	return nil
}

func runConfigTagRemove(cmd *cobra.Command, args []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	catalog, catalogPath, err := loadCatalogForRuntimeContext(runtimeCtx)
	if err != nil {
		return err
	}

	req, err := resolveConfigTagModifyRequest(
		cmd.Context(),
		args,
		runtimeCtx.Cwd,
		catalog,
		fsfind.GitRootPath,
		findGitRepoPaths,
		inspectRepoMetadata,
	)
	if err != nil {
		return err
	}

	if err := confirmConfigTagModify("remove", req); err != nil {
		return err
	}

	for _, repoSelector := range req.RepoSelectors {
		if err := fconfig.RemoveTags(catalog, repoSelector, req.Tags); err != nil {
			return err
		}
	}

	if err := fconfig.SaveCatalog(catalogPath, catalog); err != nil {
		return err
	}

	if len(req.RepoSelectors) == 1 {
		ptermSuccessMessageStyle.Printfln("tags updated for %s", req.RepoSelectors[0])
		return nil
	}

	ptermSuccessMessageStyle.Printfln("tags updated for %d repositories", len(req.RepoSelectors))
	return nil
}

func runConfigTagList(_ *cobra.Command, args []string) error {
	catalog, _, err := loadCatalogForCurrentRuntimeContext()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		selector, err := resolveConfigTagListSelector(catalog, args[0])
		if err != nil {
			return err
		}

		index, err := fconfig.ResolveRepoIndex(catalog, selector)
		if err != nil {
			return err
		}
		pterm.Printf("%s\t%s\n", catalog.Repos[index].ID, strings.Join(catalog.Repos[index].Tags, ","))
		return nil
	}

	repos := make([]fconfig.RepoEntry, 0, len(catalog.Repos))
	for _, repo := range catalog.Repos {
		if len(repo.Tags) == 0 {
			continue
		}
		repos = append(repos, repo)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].ID < repos[j].ID
	})

	for _, repo := range repos {
		pterm.Printf("%s\t%s\n", repo.ID, strings.Join(repo.Tags, ","))
	}

	return nil
}

func resolveConfigTagListSelector(catalog *fconfig.Catalog, selector string) (string, error) {
	normalizedSelector, matched, err := resolveExplicitCatalogRepoSelector(catalog, selector)
	if err != nil {
		return "", err
	}
	if matched {
		return normalizedSelector, nil
	}

	return selector, nil
}

func parseConfigTagModifyArgs(args []string, cwd string, gitRoot configTagGitRootFn) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, errors.New("requires a repository selector and at least one tag")
	}

	if len(args) == 1 {
		repoRoot, err := gitRoot(cwd)
		if err != nil {
			return "", nil, fmt.Errorf(
				"requires a repository selector and at least one tag, or run this command from a Git repository root: %w",
				err,
			)
		}

		return repoRoot, args, nil
	}

	return args[0], args[1:], nil
}

func resolveConfigTagModifyRequest(
	ctx context.Context,
	args []string,
	cwd string,
	catalog *fconfig.Catalog,
	gitRoot configTagGitRootFn,
	findRepos configTagFindReposFn,
	inspectRepo configTagInspectRepoFn,
) (configTagModifyRequest, error) {
	if len(args) == 0 {
		return configTagModifyRequest{}, errors.New("requires a repository selector and at least one tag")
	}

	if len(args) >= 2 {
		repoSelector, matched, err := resolveExplicitCatalogRepoSelector(catalog, args[0])
		if err != nil {
			return configTagModifyRequest{}, err
		}
		if matched {
			if err := validateConfigTagValues(args[1:]); err != nil {
				return configTagModifyRequest{}, err
			}

			return configTagModifyRequest{
				RepoSelectors: []string{repoSelector},
				Tags:          args[1:],
			}, nil
		}
	}

	if repoRoot, err := gitRoot(cwd); err == nil {
		if err := validateConfigTagValues(args); err != nil {
			return configTagModifyRequest{}, err
		}

		return configTagModifyRequest{
			RepoSelectors: []string{repoRoot},
			Tags:          args,
		}, nil
	}

	repoSelectors, err := resolveCatalogRepoSelectorsBySearch(ctx, cwd, catalog, findRepos, inspectRepo)
	if err != nil {
		return configTagModifyRequest{}, err
	}

	if err := validateConfigTagValues(args); err != nil {
		return configTagModifyRequest{}, err
	}

	return configTagModifyRequest{
		RepoSelectors:        repoSelectors,
		Tags:                 args,
		RequiresConfirmation: true,
	}, nil
}

func resolveExplicitCatalogRepoSelector(catalog *fconfig.Catalog, selector string) (string, bool, error) {
	if index, err := fconfig.ResolveRepoIndex(catalog, selector); err == nil {
		return catalog.Repos[index].ID, true, nil
	}

	if !looksLikeRemoteRepoURL(selector) {
		return "", false, nil
	}

	repoID, err := gitRemoteURLProjectID(selector)
	if err != nil {
		return "", false, fmt.Errorf("invalid repository URL %q: %w", selector, err)
	}

	index, err := fconfig.ResolveRepoIndex(catalog, repoID)
	if err != nil {
		return "", false, err
	}

	return catalog.Repos[index].ID, true, nil
}

func validateConfigTagValues(tags []string) error {
	for _, tag := range tags {
		if looksLikeRemoteRepoURL(tag) {
			return fmt.Errorf("tag %q must not be a repository URL", tag)
		}
	}

	return nil
}

func looksLikeRemoteRepoURL(value string) bool {
	if hasScheme(value) {
		return true
	}

	atIndex := strings.Index(value, "@")
	if atIndex <= 0 {
		return false
	}

	remainder := value[atIndex+1:]
	colonIndex := strings.Index(remainder, ":")
	if colonIndex <= 0 {
		return false
	}

	return strings.Contains(remainder[colonIndex+1:], "/")
}

func resolveCatalogRepoSelectorsBySearch(
	ctx context.Context,
	cwd string,
	catalog *fconfig.Catalog,
	findRepos configTagFindReposFn,
	inspectRepo configTagInspectRepoFn,
) ([]string, error) {
	repoPaths, err := findRepos(ctx, cwd)
	if err != nil {
		return nil, err
	}

	sort.Strings(repoPaths)

	seen := make(map[string]struct{}, len(repoPaths))
	repoSelectors := make([]string, 0, len(repoPaths))
	for _, repoPath := range repoPaths {
		index, err := fconfig.ResolveRepoIndex(catalog, repoPath)
		if err != nil {
			metadata, inspectErr := inspectRepo(repoPath)
			if inspectErr != nil || metadata.ID == "" {
				continue
			}

			index, err = fconfig.ResolveRepoIndex(catalog, metadata.ID)
			if err != nil {
				continue
			}
		}

		repoID := catalog.Repos[index].ID
		if _, ok := seen[repoID]; ok {
			continue
		}

		seen[repoID] = struct{}{}
		repoSelectors = append(repoSelectors, repoID)
	}

	if len(repoSelectors) == 0 {
		return nil, fmt.Errorf("no catalog repositories found under %s", cwd)
	}

	return repoSelectors, nil
}

func confirmConfigTagModify(action string, req configTagModifyRequest) error {
	if !req.RequiresConfirmation || configTagCmdFlags.AssumeYes {
		return nil
	}

	if isNotTerminal {
		return fmt.Errorf("tag %s requires confirmation; rerun with --yes for non-interactive use", action)
	}

	confirmMsg := configTagModifyConfirmText(action, req)

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

func configTagModifyConfirmText(action string, req configTagModifyRequest) string {
	var b strings.Builder

	fmt.Fprintf(
		&b,
		"%s tags [%s] on %d discovered repositories:\n",
		action,
		strings.Join(req.Tags, ","),
		len(req.RepoSelectors),
	)

	for _, repoSelector := range req.RepoSelectors {
		fmt.Fprintf(&b, " - %s\n", repoSelector)
	}

	b.WriteString("continue?")

	return b.String()
}
