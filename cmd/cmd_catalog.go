package cmd

import (
	"encoding/json"
	"errors"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Inspect and sync repository catalog",
}

var catalogListCmd = &cobra.Command{
	Use:   "list [repo...]",
	Short: "List cataloged repositories",
	Args:  cobra.ArbitraryArgs,
	RunE:  runCatalogList,
}

var catalogShowCmd = &cobra.Command{
	Use:   "show <repo...>",
	Short: "Show catalog entries as JSON",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCatalogShow,
}

var catalogPathsCmd = &cobra.Command{
	Use:   "paths <repo...>",
	Short: "List catalog paths for repositories",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCatalogPaths,
}

func init() {
	rootCmd.AddCommand(catalogCmd)

	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	catalogCmd.AddCommand(catalogPathsCmd)
}

func runCatalogList(_ *cobra.Command, args []string) error {
	catalog, _, err := loadCatalogForCurrentRuntimeContext()
	if err != nil {
		return err
	}

	repos, err := selectCatalogRepos(catalog, args)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		pterm.Println(repo.ID)
	}

	return nil
}

func runCatalogShow(_ *cobra.Command, args []string) error {
	catalog, _, err := loadCatalogForCurrentRuntimeContext()
	if err != nil {
		return err
	}

	repos, err := selectCatalogRepos(catalog, args)
	if err != nil {
		return err
	}

	var out any
	if len(repos) == 1 {
		out = repos[0]
	} else {
		out = repos
	}

	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	pterm.Println(string(enc))
	return nil
}

func runCatalogPaths(_ *cobra.Command, args []string) error {
	catalog, _, err := loadCatalogForCurrentRuntimeContext()
	if err != nil {
		return err
	}

	repos, err := selectCatalogRepos(catalog, args)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if len(repo.Locations) == 0 {
			pterm.Printf("%s\t\n", repo.ID)
			continue
		}

		for _, location := range repo.Locations {
			pterm.Printf("%s\t%s\n", repo.ID, location.Path)
		}
	}

	return nil
}

func loadCatalogForCurrentRuntimeContext() (*fconfig.Catalog, string, error) {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return nil, "", err
	}

	return loadCatalogForRuntimeContext(runtimeCtx)
}

func loadCatalogForRuntimeContext(runtimeCtx configRuntimeContext) (*fconfig.Catalog, string, error) {
	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return nil, "", err
	}

	return loadCatalogForEffectiveConfig(config)
}

func selectCatalogRepos(catalog *fconfig.Catalog, selectors []string) ([]fconfig.RepoEntry, error) {
	if catalog == nil {
		return nil, errors.New("nil catalog")
	}

	if len(selectors) == 0 {
		return append([]fconfig.RepoEntry(nil), catalog.Repos...), nil
	}

	seen := make(map[string]struct{}, len(selectors))
	repos := make([]fconfig.RepoEntry, 0, len(selectors))

	for _, selector := range selectors {
		repoID, err := resolveCatalogRepoSelector(catalog, selector)
		if err != nil {
			return nil, err
		}

		if _, ok := seen[repoID]; ok {
			continue
		}
		seen[repoID] = struct{}{}

		index, err := fconfig.ResolveRepoIndex(catalog, repoID)
		if err != nil {
			return nil, err
		}

		repos = append(repos, catalog.Repos[index])
	}

	return repos, nil
}

func resolveCatalogRepoSelector(catalog *fconfig.Catalog, selector string) (string, error) {
	normalizedSelector, matched, err := resolveExplicitCatalogRepoSelector(catalog, selector)
	if err != nil {
		return "", err
	}
	if matched {
		return normalizedSelector, nil
	}

	index, err := fconfig.ResolveRepoIndex(catalog, selector)
	if err != nil {
		return "", err
	}

	return catalog.Repos[index].ID, nil
}
