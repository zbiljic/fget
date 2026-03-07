package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	configTagGitRootFn   func(path string) (string, error)
	configTagFindReposFn func(ctx context.Context, roots ...string) ([]string, error)
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

	configCmd.AddCommand(configTagCmd)
	configTagCmd.AddCommand(configTagAddCmd)
	configTagCmd.AddCommand(configTagRemoveCmd)
	configTagCmd.AddCommand(configTagListCmd)
}

func runConfigTagAdd(cmd *cobra.Command, args []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	catalog, catalogPath, err := loadCatalogForTagCommandWithRuntimeContext(runtimeCtx)
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

	catalog, catalogPath, err := loadCatalogForTagCommandWithRuntimeContext(runtimeCtx)
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
	catalog, _, err := loadCatalogForTagCommand()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		index, err := fconfig.ResolveRepoIndex(catalog, args[0])
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
) (configTagModifyRequest, error) {
	if len(args) == 0 {
		return configTagModifyRequest{}, errors.New("requires a repository selector and at least one tag")
	}

	if len(args) >= 2 {
		if _, err := fconfig.ResolveRepoIndex(catalog, args[0]); err == nil {
			return configTagModifyRequest{
				RepoSelectors: []string{args[0]},
				Tags:          args[1:],
			}, nil
		}
	}

	if repoRoot, err := gitRoot(cwd); err == nil {
		return configTagModifyRequest{
			RepoSelectors: []string{repoRoot},
			Tags:          args,
		}, nil
	}

	repoSelectors, err := resolveCatalogRepoSelectorsBySearch(ctx, cwd, catalog, findRepos)
	if err != nil {
		return configTagModifyRequest{}, err
	}

	return configTagModifyRequest{
		RepoSelectors:        repoSelectors,
		Tags:                 args,
		RequiresConfirmation: true,
	}, nil
}

func resolveCatalogRepoSelectorsBySearch(
	ctx context.Context,
	cwd string,
	catalog *fconfig.Catalog,
	findRepos configTagFindReposFn,
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
			continue
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
		return fmt.Errorf("config tag %s requires confirmation; rerun with --yes for non-interactive use", action)
	}

	confirmMsg := fmt.Sprintf(
		"%s tags [%s] on %d discovered repositories",
		action,
		strings.Join(req.Tags, ","),
		len(req.RepoSelectors),
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

func loadCatalogForTagCommand() (*fconfig.Catalog, string, error) {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return nil, "", err
	}

	return loadCatalogForTagCommandWithRuntimeContext(runtimeCtx)
}

func loadCatalogForTagCommandWithRuntimeContext(runtimeCtx configRuntimeContext) (*fconfig.Catalog, string, error) {
	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return nil, "", err
	}

	if _, err := os.Stat(config.Catalog.Path); err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("catalog does not exist at %s; run `fget config sync` first", config.Catalog.Path)
		}
		return nil, "", err
	}

	catalog, err := fconfig.LoadCatalog(config.Catalog.Path)
	if err != nil {
		return nil, "", err
	}

	return catalog, config.Catalog.Path, nil
}
