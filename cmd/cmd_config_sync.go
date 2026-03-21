package cmd

import (
	"context"
	"fmt"
	"time"

	art "github.com/plar/go-adaptive-radix-tree/v2"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/fsfind"
)

var configSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync repository catalog from configured roots",
	Args:  cobra.ArbitraryArgs,
	RunE:  runConfigSync,
}

type configSyncOptions struct {
	Roots   []string
	Prune   bool
	Silent  bool
	Workers uint16
}

type syncRepoMetadata struct {
	ID        string
	RemoteURL string
}

var configSyncCmdFlags = configSyncOptions{}

const (
	configSyncProgressUpdateInterval = 250 * time.Millisecond
	configSyncDefaultMaxWorkers      = 32
)

func init() {
	configSyncCmd.Flags().StringArrayVar(&configSyncCmdFlags.Roots, "root", nil, "Root directories to scan (overrides configured roots)")
	configSyncCmd.Flags().BoolVar(&configSyncCmdFlags.Prune, "prune", false, "Remove catalog repositories that are not found during sync")
	configSyncCmd.Flags().BoolVar(&configSyncCmdFlags.Silent, "silent", false, "Suppress live progress output and print only the final summary")
	configSyncCmd.Flags().Uint16VarP(&configSyncCmdFlags.Workers, "workers", "j", configSyncDefaultMaxWorkers, "Set the maximum number of workers to use")

	configCmd.AddCommand(configSyncCmd)
}

func runConfigSync(cmd *cobra.Command, args []string) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return err
	}

	argRoots, err := parseConfigSyncArgs(args)
	if err != nil {
		return err
	}

	roots, err := resolveSyncRoots(
		configSyncCmdFlags.Roots,
		argRoots,
		config.Roots,
		runtimeCtx.Cwd,
		runtimeCtx.HomeDir,
		normalizeConfigRoots,
	)
	if err != nil {
		return err
	}

	catalog, err := fconfig.LoadCatalog(config.Catalog.Path)
	if err != nil {
		return err
	}

	var progressReporter *configSyncProgressReporter
	if configSyncProgressEnabled(configSyncCmdFlags.Silent, isInteractiveWriter(dynamicOutput)) {
		spinner, err := pterm.DefaultSpinner.
			WithWriter(dynamicOutput).
			WithRemoveWhenDone(true).
			Start(formatConfigSyncProgressText(0, 0))
		if err != nil {
			return err
		}
		defer spinner.Stop() //nolint:errcheck

		progressReporter = newConfigSyncProgressReporter(spinner, configSyncProgressUpdateInterval)
	}

	err = fconfig.SyncCatalog(
		cmd.Context(),
		catalog,
		fconfig.SyncOptions{
			Roots:    roots,
			Prune:    configSyncCmdFlags.Prune,
			Workers:  int(configSyncCmdFlags.Workers),
			Progress: progressReporter.Update,
		},
		func(roots ...string) ([]string, error) {
			return findGitRepoPaths(cmd.Context(), roots...)
		},
		inspectRepoMetadata,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	if err := fconfig.SaveCatalog(config.Catalog.Path, catalog); err != nil {
		return err
	}

	ptermSuccessMessageStyle.Printfln("catalog synced: %d repositories (%s)", len(catalog.Repos), config.Catalog.Path)

	return nil
}

func configSyncProgressEnabled(silent, interactive bool) bool {
	return !silent && interactive
}

func parseConfigSyncArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return []string{}, nil
	}

	roots := make([]string, 0, len(args))
	for _, arg := range args {
		path, err := fsfind.DirAbsPath(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid root %q: %w", arg, err)
		}
		roots = append(roots, path)
	}

	return roots, nil
}

func normalizeConfigRoots(roots []string, homeDir string) ([]string, error) {
	out := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))

	for _, root := range roots {
		root = expandHomePath(root, homeDir)

		path, err := fsfind.DirAbsPath(root)
		if err != nil {
			return nil, fmt.Errorf("invalid configured root %q: %w", root, err)
		}

		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}

	return out, nil
}

type normalizeRootsFn func([]string, string) ([]string, error)

func resolveSyncRoots(
	flagRoots []string,
	argRoots []string,
	configRoots []string,
	cwd string,
	homeDir string,
	normalize normalizeRootsFn,
) ([]string, error) {
	switch {
	case len(flagRoots) > 0:
		return normalize(flagRoots, homeDir)
	case len(argRoots) > 0:
		return argRoots, nil
	case len(configRoots) > 0:
		return normalize(configRoots, homeDir)
	default:
		return []string{cwd}, nil
	}
}

func findGitRepoPaths(ctx context.Context, roots ...string) ([]string, error) {
	tree, err := fsfind.GitDirectoriesTreeContext(ctx, roots...)
	if err != nil {
		return nil, err
	}

	repoPaths := make([]string, 0, tree.Size())
	tree.ForEach(func(node art.Node) bool {
		repoPaths = append(repoPaths, string(node.Key()))
		return true
	})

	return repoPaths, nil
}

func inspectRepoMetadata(repoPath string) (fconfig.RepoMetadata, error) {
	meta, err := inspectSyncRepoMetadata(repoPath)
	if err != nil {
		return fconfig.RepoMetadata{}, err
	}

	return fconfig.RepoMetadata{
		ID:        meta.ID,
		Path:      repoPath,
		RemoteURL: meta.RemoteURL,
	}, nil
}

func inspectSyncRepoMetadata(repoPath string) (syncRepoMetadata, error) {
	remoteURL, err := gitRemoteConfigURL(repoPath)
	if err != nil {
		return syncRepoMetadata{}, err
	}

	repoID, err := gitRemoteURLProjectID(remoteURL.String())
	if err != nil {
		return syncRepoMetadata{}, err
	}

	return syncRepoMetadata{
		ID:        repoID,
		RemoteURL: remoteURL.String(),
	}, nil
}

type configSyncProgressReporter struct {
	spinner        *pterm.SpinnerPrinter
	updateInterval time.Duration
	lastRenderedAt time.Time
}

func newConfigSyncProgressReporter(spinner *pterm.SpinnerPrinter, updateInterval time.Duration) *configSyncProgressReporter {
	return &configSyncProgressReporter{
		spinner:        spinner,
		updateInterval: updateInterval,
	}
}

func (r *configSyncProgressReporter) Update(processed, total int) {
	if r == nil || r.spinner == nil {
		return
	}

	now := time.Now()
	if total > 0 && processed != total && !r.lastRenderedAt.IsZero() && now.Sub(r.lastRenderedAt) < r.updateInterval {
		return
	}

	r.spinner.UpdateText(formatConfigSyncProgressText(processed, total))
	r.lastRenderedAt = now
}

func formatConfigSyncProgressText(processed, total int) string {
	if total == 0 {
		return "finding repositories..."
	}

	return fmt.Sprintf("syncing catalog: %d/%d", processed, total)
}
