package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
)

var configTagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Manage repository tags",
}

var configTagAddCmd = &cobra.Command{
	Use:   "add <repo> <tag...>",
	Short: "Add one or more tags to a repository",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runConfigTagAdd,
}

var configTagRemoveCmd = &cobra.Command{
	Use:   "remove <repo> <tag...>",
	Short: "Remove one or more tags from a repository",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runConfigTagRemove,
}

var configTagListCmd = &cobra.Command{
	Use:   "list [repo]",
	Short: "List tags",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigTagList,
}

func init() {
	configCmd.AddCommand(configTagCmd)
	configTagCmd.AddCommand(configTagAddCmd)
	configTagCmd.AddCommand(configTagRemoveCmd)
	configTagCmd.AddCommand(configTagListCmd)
}

func runConfigTagAdd(_ *cobra.Command, args []string) error {
	repoSelector, tags, err := parseConfigTagModifyArgs(args)
	if err != nil {
		return err
	}

	catalog, catalogPath, err := loadCatalogForTagCommand()
	if err != nil {
		return err
	}

	if err := fconfig.AddTags(catalog, repoSelector, tags); err != nil {
		return err
	}

	if err := fconfig.SaveCatalog(catalogPath, catalog); err != nil {
		return err
	}

	ptermSuccessMessageStyle.Printfln("tags updated for %s", repoSelector)
	return nil
}

func runConfigTagRemove(_ *cobra.Command, args []string) error {
	repoSelector, tags, err := parseConfigTagModifyArgs(args)
	if err != nil {
		return err
	}

	catalog, catalogPath, err := loadCatalogForTagCommand()
	if err != nil {
		return err
	}

	if err := fconfig.RemoveTags(catalog, repoSelector, tags); err != nil {
		return err
	}

	if err := fconfig.SaveCatalog(catalogPath, catalog); err != nil {
		return err
	}

	ptermSuccessMessageStyle.Printfln("tags updated for %s", repoSelector)
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

func parseConfigTagModifyArgs(args []string) (string, []string, error) {
	if len(args) < 2 {
		return "", nil, errors.New("requires a repository selector and at least one tag")
	}

	return args[0], args[1:], nil
}

func loadCatalogForTagCommand() (*fconfig.Catalog, string, error) {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return nil, "", err
	}

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
