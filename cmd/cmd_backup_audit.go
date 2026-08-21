package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fbackup"
	"github.com/zbiljic/fget/pkg/fconfig"
	"github.com/zbiljic/fget/pkg/fsfind"
)

const backupAuditRemoteTimeout = 10 * time.Second

type backupAuditFlags struct {
	CatalogPath   string
	Output        string
	VerifyRemotes bool
	Workers       int
}

type backupAuditCatalog struct {
	View     *fconfig.Catalog
	Identity *fbackup.CatalogIdentity
}

type backupCatalogIndex struct {
	byPath      map[string]fconfig.RepoEntry
	byRemoteURL map[string]fconfig.RepoEntry
}

var (
	backupAuditCmdFlags = backupAuditFlags{
		Output:  "-",
		Workers: int(poolDefaultMaxWorkers),
	}
	backupAuditFindReposFn        = fsfind.GitDirectoriesStrictContext
	backupAuditInspectRepoFn      = inspectBackupAuditRepository
	backupAuditNowFn              = func() time.Time { return time.Now().UTC() }
	backupAuditLoadCatalogFn      = loadBackupAuditCatalog
	backupAuditGitRunnerFactoryFn = func() backupGitRunner { return backupGitCLI{} }
)

var backupAuditCmd = &cobra.Command{
	Use:   "audit [root...]",
	Short: "Write a read-only backup audit manifest as JSON",
	Args:  cobra.ArbitraryArgs,
	RunE:  runBackupAudit,
}

func init() {
	backupCmd.AddCommand(backupAuditCmd)

	backupAuditCmd.Flags().StringVar(&backupAuditCmdFlags.CatalogPath, "catalog", "", "Explicit catalog file")
	backupAuditCmd.Flags().StringVar(&backupAuditCmdFlags.Output, "output", "-", "Output file path, or - for stdout")
	backupAuditCmd.Flags().BoolVar(&backupAuditCmdFlags.VerifyRemotes, "verify-remotes", false, "Verify remote reachability with git ls-remote")
	backupAuditCmd.Flags().IntVarP(&backupAuditCmdFlags.Workers, "workers", "j", int(poolDefaultMaxWorkers), "Set the maximum number of workers to use")
}

func runBackupAudit(cmd *cobra.Command, args []string) error {
	if err := validateBackupAuditFlags(backupAuditCmdFlags); err != nil {
		return err
	}

	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return err
	}

	roots, err := resolveBackupAuditRoots(args, config, runtimeCtx)
	if err != nil {
		return err
	}

	repoPaths, err := backupAuditFindReposFn(cmd.Context(), roots...)
	if err != nil {
		return err
	}

	sort.Strings(repoPaths)
	if err := ensureBackupAuditOutputOutsideRepos(backupAuditCmdFlags.Output, repoPaths); err != nil {
		return err
	}

	catalog, err := backupAuditLoadCatalogFn(config, runtimeCtx, backupAuditCmdFlags.CatalogPath)
	if err != nil {
		return err
	}
	index := buildBackupCatalogIndex(catalog)

	records, err := collectBackupAuditRecords(cmd.Context(), repoPaths, backupAuditCmdFlags, index)
	if err != nil {
		return err
	}

	manifest := fbackup.Manifest{
		Version:      fbackup.ManifestVersion,
		GeneratedAt:  backupAuditNowFn(),
		Roots:        sortedUniquePaths(roots),
		Repositories: records,
	}
	if catalog != nil {
		manifest.Catalog = catalog.Identity
	}

	write := func(w io.Writer) error {
		return writeBackupAuditManifest(w, manifest)
	}
	if backupAuditCmdFlags.Output == "-" {
		return write(cmd.OutOrStdout())
	}

	return writeBackupAuditFile(backupAuditCmdFlags.Output, write)
}

func validateBackupAuditFlags(flags backupAuditFlags) error {
	if strings.TrimSpace(flags.Output) == "" {
		return errors.New("--output must not be empty")
	}
	if flags.Workers <= 0 {
		return errors.New("--workers must be greater than zero")
	}
	return nil
}

func resolveBackupAuditRoots(args []string, config *fconfig.EffectiveConfig, runtimeCtx configRuntimeContext) ([]string, error) {
	argRoots, err := parseConfigSyncArgs(args)
	if err != nil {
		return nil, err
	}

	return resolveSyncRoots(nil, argRoots, config.Roots, nil, runtimeCtx.Cwd, runtimeCtx.HomeDir, normalizeConfigRoots)
}

func loadBackupAuditCatalog(config *fconfig.EffectiveConfig, runtimeCtx configRuntimeContext, explicitPath string) (*backupAuditCatalog, error) {
	if strings.TrimSpace(explicitPath) != "" {
		catalog, digest, err := loadCatalogForExport(catalogExportFlags{CatalogPath: explicitPath})
		if err != nil {
			return nil, err
		}
		absolutePath, err := filepath.Abs(explicitPath)
		if err != nil {
			return nil, err
		}
		return &backupAuditCatalog{
			View: catalog,
			Identity: &fbackup.CatalogIdentity{
				Path:   absolutePath,
				Digest: digest,
			},
		}, nil
	}

	if config == nil || strings.TrimSpace(config.Catalog.Path) == "" {
		return nil, nil
	}

	set, err := loadCatalogSetForEffectiveConfig(config, runtimeCtx.HomeDir)
	if err != nil {
		if backupAuditMissingCatalog(err) {
			return nil, nil
		}
		return nil, err
	}

	digest, err := catalogExportDigest(set.View)
	if err != nil {
		return nil, err
	}
	return &backupAuditCatalog{
		View: set.View,
		Identity: &fbackup.CatalogIdentity{
			Path:   config.Catalog.Path,
			Digest: digest,
		},
	}, nil
}

func backupAuditMissingCatalog(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "run `fget catalog sync` first")
}

func buildBackupCatalogIndex(catalog *backupAuditCatalog) *backupCatalogIndex {
	if catalog == nil || catalog.View == nil {
		return nil
	}

	index := &backupCatalogIndex{
		byPath:      make(map[string]fconfig.RepoEntry),
		byRemoteURL: make(map[string]fconfig.RepoEntry),
	}

	for _, repo := range catalog.View.Repos {
		index.byRemoteURL[repo.RemoteURL] = repo
		for _, location := range repo.Locations {
			index.byPath[filepath.Clean(location.Path)] = repo
		}
	}

	return index
}

func collectBackupAuditRecords(
	ctx context.Context,
	repoPaths []string,
	flags backupAuditFlags,
	index *backupCatalogIndex,
) ([]fbackup.RepositoryEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resultPool := pond.NewResultPool[fbackup.RepositoryEntry](
		flags.Workers,
		pond.WithQueueSize(poolDefaultMaxCapacity),
	)
	defer resultPool.StopAndWait()

	group := resultPool.NewGroupContext(ctx)
	for _, repoPath := range repoPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		repoPath := repoPath
		group.SubmitErr(func() (fbackup.RepositoryEntry, error) {
			return backupAuditInspectRepoFn(ctx, repoPath, flags, index)
		})
	}

	results, err := group.Wait()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	records := make([]fbackup.RepositoryEntry, 0, len(results))
	records = append(records, results...)

	sort.Slice(records, func(i, j int) bool {
		if records[i].ID == records[j].ID {
			return records[i].Path < records[j].Path
		}
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func inspectBackupAuditRepository(
	ctx context.Context,
	repoPath string,
	flags backupAuditFlags,
	index *backupCatalogIndex,
) (fbackup.RepositoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return fbackup.RepositoryEntry{}, err
	}

	runner := backupAuditGitRunnerFactoryFn()
	localProbe := inspectBackupLocalRepository(ctx, repoPath, runner)
	if err := ctx.Err(); err != nil {
		return fbackup.RepositoryEntry{}, err
	}
	repoEntry := backupCatalogLookup(index, repoPath, localProbe.RemoteURL)

	repoID := filepath.Clean(repoPath)
	remoteURL := localProbe.RemoteURL
	if repoEntry != nil {
		repoID = repoEntry.ID
		if remoteURL == "" {
			remoteURL = repoEntry.RemoteURL
		}
	}
	if remoteURL != "" {
		if derivedID, err := gitRemoteURLProjectID(remoteURL); err == nil {
			repoID = derivedID
		}
	}

	remoteState := fbackup.RemoteStateUnchecked
	remoteReason := ""
	if flags.VerifyRemotes {
		if localProbe.HasOrigin {
			remoteCtx, cancel := context.WithTimeout(ctx, backupAuditRemoteTimeout)
			remoteState, remoteReason = verifyBackupRemote(remoteCtx, repoPath, runner)
			cancel()
		} else {
			remoteState = fbackup.RemoteStateMissing
			remoteReason = "origin remote not configured"
		}
	}

	record := fbackup.BuildRepositoryEntry(fbackup.RepositoryProbe{
		ID:                   repoID,
		Path:                 filepath.Clean(repoPath),
		RemoteURL:            remoteURL,
		Git:                  localProbe.Git,
		TrackedDirtyCount:    localProbe.TrackedDirtyCount,
		UntrackedCount:       localProbe.UntrackedCount,
		UntrackedBytes:       localProbe.UntrackedBytes,
		LocalOnlyCommitCount: localProbe.LocalOnlyCommitCount,
		RemoteState:          remoteState,
		RemoteReason:         remoteReason,
		HasLFSAttributes:     localProbe.HasLFSAttributes,
		HasLocalLFSObjects:   localProbe.HasLocalLFSObjects,
		HasSubmodules:        localProbe.HasSubmodules,
		EstimatedSourceBytes: localProbe.EstimatedSourceBytes,
		Errors:               append([]fbackup.RepositoryError(nil), localProbe.Errors...),
	})
	if record.ID == "" {
		record.ID = filepath.Clean(repoPath)
	}

	return record, nil
}

func backupCatalogLookup(index *backupCatalogIndex, repoPath, remoteURL string) *fconfig.RepoEntry {
	if index == nil {
		return nil
	}

	if repo, ok := index.byPath[filepath.Clean(repoPath)]; ok {
		return &repo
	}
	if remoteURL != "" {
		if repo, ok := index.byRemoteURL[remoteURL]; ok {
			return &repo
		}
	}

	return nil
}

func ensureBackupAuditOutputOutsideRepos(outputPath string, repoPaths []string) error {
	if outputPath == "-" {
		return nil
	}

	absolutePath, err := normalizeBackupAuditOutputPath(outputPath)
	if err != nil {
		return err
	}
	for _, repoPath := range repoPaths {
		inside, err := isPathWithin(repoPath, absolutePath)
		if err != nil {
			return err
		}
		if inside {
			return fmt.Errorf("output path %q must be outside audited repository %q", absolutePath, repoPath)
		}
	}

	return nil
}

func normalizeBackupAuditOutputPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	if normalizedPath, err := filepath.EvalSymlinks(path); err == nil {
		return normalizedPath, nil
	}

	parent := filepath.Dir(path)
	normalizedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return path, nil
	}

	return filepath.Join(normalizedParent, filepath.Base(path)), nil
}

func writeBackupAuditManifest(w io.Writer, manifest fbackup.Manifest) error {
	return outputJSON(w, manifest)
}

func writeBackupAuditFile(path string, write func(io.Writer) error) (err error) {
	return writeAtomicOutputFile(path, ".fget-backup-audit-*", write)
}

func sortedUniquePaths(values []string) []string {
	cleaned := make([]string, len(values))
	for index, value := range values {
		cleaned[index] = filepath.Clean(value)
	}
	return sortedUnique(cleaned)
}
