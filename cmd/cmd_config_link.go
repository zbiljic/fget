package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
)

var configLinkCmd = &cobra.Command{
	Use:   "link",
	Short: "Manage repository link projections",
}

var configLinkSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync repository symlinks from catalog tags",
	Args:  cobra.NoArgs,
	RunE:  runConfigLinkSync,
}

func init() {
	configCmd.AddCommand(configLinkCmd)
	configLinkCmd.AddCommand(configLinkSyncCmd)
}

func runConfigLinkSync(_ *cobra.Command, _ []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	config, err := resolveLinkConfigForRuntimeContext(runtimeCtx)
	if err != nil {
		return err
	}

	catalog, _, err := loadCatalogForEffectiveConfig(config)
	if err != nil {
		return err
	}

	targets, problems := fconfig.ResolveLinkTargets(catalog, *config.Link)
	result, syncErr := fconfig.SyncLinks(config.Link.Root, targets)

	skippedCount := len(problems) + len(result.Skipped)
	if skippedCount == 0 {
		ptermSuccessMessageStyle.Printfln(
			"links synced: %d created, %d updated, %d removed",
			result.Created,
			result.Updated,
			result.Removed,
		)
		return nil
	}

	ptermWarningMessageStyle.Printfln(
		"links synced with warnings: %d created, %d updated, %d removed, %d skipped",
		result.Created,
		result.Updated,
		result.Removed,
		skippedCount,
	)
	printLinkProblems(append(problems, result.Skipped...))

	return errors.Join(syncErr, joinCommandLinkProblems(problems))
}

func resolveLinkConfigForRuntimeContext(runtimeCtx configRuntimeContext) (*fconfig.EffectiveConfig, error) {
	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return nil, err
	}
	if config.Link == nil {
		return nil, errors.New("no link configuration found in discovered fget.yaml files")
	}
	return config, nil
}

func loadCatalogForEffectiveConfig(config *fconfig.EffectiveConfig) (*fconfig.Catalog, string, error) {
	if config == nil {
		return nil, "", errors.New("nil effective config")
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

func printLinkProblems(problems []fconfig.LinkProblem) {
	for _, problem := range problems {
		if problem.RepoID == "" {
			ptermErrorMessageStyle.Printfln("%v", problem.Err)
			continue
		}
		ptermErrorMessageStyle.Printfln("%s: %v", problem.RepoID, problem.Err)
	}
}

func joinCommandLinkProblems(problems []fconfig.LinkProblem) error {
	if len(problems) == 0 {
		return nil
	}

	errs := make([]error, 0, len(problems))
	for _, problem := range problems {
		if problem.RepoID == "" {
			errs = append(errs, problem.Err)
			continue
		}
		errs = append(errs, fmt.Errorf("%s: %w", problem.RepoID, problem.Err))
	}
	return errors.Join(errs...)
}
