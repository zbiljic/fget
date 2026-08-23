package fbackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/zbiljic/fget/pkg/gitinspect"
	"github.com/zbiljic/fget/pkg/giturl"
)

// RestoreOptions controls a restore. RepositoryIDs is empty when all entries
// in the verified audit manifest should be restored.
type RestoreOptions struct {
	Backup        string
	Destination   string
	RepositoryIDs []string
	DryRun        bool
	Progress      func(repositoryID, status string)
	GitRunner     gitinspect.Runner
}

// RestoreReport is a deterministic summary of a restore operation.
type RestoreReport struct {
	Planned  []string
	Restored []string
	Skipped  []string
	Failed   []string
	Errors   map[string]string
}

const restoreCloneTimeout = 5 * time.Minute

// Restore verifies the complete backup before publishing any repository.
func Restore(ctx context.Context, options RestoreOptions) error {
	_, err := RestoreWithReport(ctx, options)
	return err
}

// RestoreWithReport restores selected repositories and returns stable counts.
func RestoreWithReport(ctx context.Context, options RestoreOptions) (RestoreReport, error) {
	report := RestoreReport{Errors: map[string]string{}}
	options, backup, destination, metadata, entries, err := prepareRestoreOptions(ctx, options)
	if err != nil {
		return report, err
	}
	if options.DryRun {
		return planRestore(options, destination, entries, report)
	}
	state, err := openRestoreDestination(destination, backup, metadata, entries)
	if err != nil {
		return report, err
	}
	return restoreEntries(ctx, options, backup, destination, metadata.Artifacts, entries, state, report)
}

func prepareRestoreOptions(
	ctx context.Context,
	options RestoreOptions,
) (RestoreOptions, string, string, BackupMetadata, []RepositoryEntry, error) {
	var metadata BackupMetadata
	if strings.TrimSpace(options.Backup) == "" || strings.TrimSpace(options.Destination) == "" {
		return options, "", "", metadata, nil, errors.New("backup and destination are required")
	}
	if options.GitRunner == nil {
		options.GitRunner = gitinspect.CLIRunner{}
	}
	backup, err := filepath.Abs(options.Backup)
	if err != nil {
		return options, "", "", metadata, nil, err
	}
	destination, err := filepath.Abs(options.Destination)
	if err != nil {
		return options, "", "", metadata, nil, err
	}
	// Deep verification is intentionally first: no destination repository path
	// is created until every indexed artifact has passed its checksum and shape checks.
	metadata, err = readVerifiedBackup(ctx, backup, true, options.GitRunner)
	if err != nil {
		return options, "", "", metadata, nil, fmt.Errorf("verify backup: %w", err)
	}
	entries, err := selectRestoreEntries(metadata.Manifest, options.RepositoryIDs)
	if err != nil {
		return options, "", "", metadata, nil, err
	}
	return options, backup, destination, metadata, entries, nil
}

func planRestore(options RestoreOptions, destination string, entries []RepositoryEntry, report RestoreReport) (RestoreReport, error) {
	if err := validateRestoreDestination(destination, entries, true); err != nil {
		return report, err
	}
	for _, entry := range entries {
		report.Planned = append(report.Planned, entry.ID)
		if options.Progress != nil {
			options.Progress(entry.ID, "planned")
		}
	}
	return report, nil
}

func openRestoreDestination(destination, backup string, metadata BackupMetadata, entries []RepositoryEntry) (restoreState, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return restoreState{}, err
	}
	state, err := readRestoreState(destination, backup, metadata)
	if err != nil {
		return state, err
	}
	selectedIDs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		selectedIDs[entry.ID] = true
	}
	for _, id := range append(append([]string{}, state.Completed...), mapKeys(state.Failed)...) {
		if !selectedIDs[id] {
			return state, fmt.Errorf("restore state contains unselected repository %q", id)
		}
	}
	if err := validateRestoreDestination(destination, entries, false); err != nil {
		// A valid resume state permits only already-published repository paths and
		// the state file; arbitrary pre-existing files remain forbidden.
		if resumeErr := validateResumeDestination(destination, state); resumeErr != nil {
			return state, resumeErr
		}
	}
	return state, nil
}

func restoreEntries(
	ctx context.Context,
	options RestoreOptions,
	backup, destination string,
	artifacts []ArtifactRecord,
	entries []RepositoryEntry,
	state restoreState,
	report RestoreReport,
) (RestoreReport, error) {
	completed := make(map[string]bool, len(state.Completed))
	for _, id := range state.Completed {
		completed[id] = true
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		target := filepath.Join(destination, filepath.FromSlash(entry.ID))
		if completed[entry.ID] {
			if err := validateRestoredRepository(ctx, target, entry, options.GitRunner); err != nil {
				return report, fmt.Errorf("resume repository %q: %w", entry.ID, err)
			}
			fingerprint, fingerprintErr := fingerprintRepository(target)
			if fingerprintErr != nil || state.Fingerprints == nil || state.Fingerprints[entry.ID] != fingerprint {
				return report, fmt.Errorf("resume repository %q: destination fingerprint mismatch", entry.ID)
			}
			report.Skipped = append(report.Skipped, entry.ID)
			continue
		}
		if options.Progress != nil {
			options.Progress(entry.ID, "starting")
		}
		if err := restoreRepository(ctx, backup, destination, artifacts, entry, options.GitRunner); err != nil {
			report.Failed = append(report.Failed, entry.ID)
			report.Errors[entry.ID] = err.Error()
			if state.Failed == nil {
				state.Failed = map[string]string{}
			}
			state.Failed[entry.ID] = err.Error()
			cleanupEmptyRestoreAncestors(destination, filepath.Dir(target))
			if writeErr := writeJSONAtomic(filepath.Join(destination, restoreStateFile), state, 0o644, ".fget-restore-state-*.tmp"); writeErr != nil {
				return report, writeErr
			}
			if options.Progress != nil {
				options.Progress(entry.ID, "failed")
			}
			continue
		}
		fingerprint, err := fingerprintRepository(target)
		if err != nil {
			return report, fmt.Errorf("fingerprint restored repository %q: %w", entry.ID, err)
		}
		state.Completed = append(state.Completed, entry.ID)
		sort.Strings(state.Completed)
		if state.Fingerprints == nil {
			state.Fingerprints = map[string]string{}
		}
		state.Fingerprints[entry.ID] = fingerprint
		delete(state.Failed, entry.ID)
		if err := writeJSONAtomic(filepath.Join(destination, restoreStateFile), state, 0o644, ".fget-restore-state-*.tmp"); err != nil {
			return report, err
		}
		report.Restored = append(report.Restored, entry.ID)
		if options.Progress != nil {
			options.Progress(entry.ID, "complete")
		}
	}
	if len(report.Failed) != 0 {
		return report, fmt.Errorf("restore failed for %d repository(ies)", len(report.Failed))
	}
	return report, nil
}

func selectRestoreEntries(manifest Manifest, selected []string) ([]RepositoryEntry, error) {
	byID := make(map[string]RepositoryEntry, len(manifest.Repositories))
	for _, entry := range manifest.Repositories {
		byID[entry.ID] = entry
	}
	if len(selected) == 0 {
		entries := append([]RepositoryEntry(nil), manifest.Repositories...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		return entries, nil
	}
	entries := make([]RepositoryEntry, 0, len(selected))
	seen := map[string]bool{}
	for _, id := range selected {
		if seen[id] {
			continue
		}
		seen[id] = true
		entry, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("repository %q is not in backup", id)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

func restoreRepository(ctx context.Context, backup, destination string, artifacts []ArtifactRecord, entry RepositoryEntry, runner gitinspect.Runner) error {
	target := filepath.Join(destination, filepath.FromSlash(entry.ID))
	parent := filepath.Dir(target)
	relParent, err := filepath.Rel(destination, parent)
	if err != nil {
		return err
	}
	if err := ensureSafeDirectory(destination, filepath.ToSlash(relParent)); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".fget-restore-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	switch entry.Classification {
	case ClassificationRecloneable:
		if err := cloneRepository(ctx, entry.RemoteURL, temporary, runner, false); err != nil {
			return err
		}
	case ClassificationDelta:
		if err := cloneRepository(ctx, entry.RemoteURL, temporary, runner, true); err != nil {
			record, _, ok := findArtifactPath(backup, artifacts, entry.ID, "bundle")
			if !ok {
				return fmt.Errorf("clone %q: %w", entry.ID, err)
			}
			if _, bundleErr := runner.Run(ctx, "", "bundle", "verify", record); bundleErr != nil {
				return fmt.Errorf("verify bundle for %q: %w", entry.ID, bundleErr)
			}
			if err := os.RemoveAll(temporary); err != nil {
				return err
			}
			if err := os.MkdirAll(temporary, 0o700); err != nil {
				return err
			}
			cloneCtx, cloneCancel := context.WithTimeout(ctx, restoreCloneTimeout)
			_, bundleErr := runner.Run(cloneCtx, "", "clone", "--no-checkout", "--", record, temporary)
			cloneCancel()
			if bundleErr != nil {
				return fmt.Errorf("clone %q from bundle: %w", entry.ID, bundleErr)
			}
		}
		record, _, ok := findArtifactPath(backup, artifacts, entry.ID, "bundle")
		if !ok {
			return errors.New("bundle artifact missing")
		}
		if _, bundleErr := runner.Run(ctx, "", "bundle", "verify", record); bundleErr != nil {
			return fmt.Errorf("verify bundle for %q: %w", entry.ID, bundleErr)
		}
		if _, detachErr := runner.Run(ctx, temporary, "checkout", "--detach"); detachErr != nil {
			return fmt.Errorf("prepare bundle refs for %q: %w", entry.ID, detachErr)
		}
		if _, bundleErr := runner.Run(ctx, temporary, "fetch", record, "+refs/*:refs/*"); bundleErr != nil {
			return fmt.Errorf("import bundle refs for %q: %w", entry.ID, bundleErr)
		}
		if origin := giturl.Sanitize(entry.RemoteURL); origin != "" {
			if _, bundleErr := runner.Run(ctx, temporary, "remote", "set-url", "origin", origin); bundleErr != nil {
				return fmt.Errorf("set origin for %q: %w", entry.ID, bundleErr)
			}
		}
		if err := checkoutManifestHead(ctx, temporary, entry, runner); err != nil {
			return err
		}
		if err := applyDelta(ctx, backup, artifacts, temporary, entry, runner); err != nil {
			return err
		}
	case ClassificationFull:
		record, _, ok := findArtifactPath(backup, artifacts, entry.ID, "full")
		if !ok {
			return errors.New("full artifact missing")
		}
		if err := extractTar(record, temporary); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported repository classification %q", entry.Classification)
	}
	if err := validateRestoredRepository(ctx, temporary, entry, runner); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("destination repository %q already exists", entry.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporary, target)
}

func cloneRepository(ctx context.Context, source, destination string, runner gitinspect.Runner, noCheckout bool) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("remote URL is empty")
	}
	cloneCtx, cancel := context.WithTimeout(ctx, restoreCloneTimeout)
	defer cancel()
	args := []string{"clone"}
	if noCheckout {
		args = append(args, "--no-checkout")
	}
	args = append(args, "--", giturl.Sanitize(source), destination)
	if _, err := runner.Run(cloneCtx, "", args...); err != nil {
		return err
	}
	return nil
}

func checkoutManifestHead(ctx context.Context, repo string, entry RepositoryEntry, runner gitinspect.Runner) error {
	if entry.Git.Head.Ref != "" {
		const headPrefix = "refs/heads/"
		if !strings.HasPrefix(entry.Git.Head.Ref, headPrefix) {
			return fmt.Errorf("unsupported symbolic HEAD %q", entry.Git.Head.Ref)
		}
		branch := strings.TrimPrefix(entry.Git.Head.Ref, headPrefix)
		if _, err := runner.Run(ctx, repo, "checkout", "--force", "-B", branch, entry.Git.Head.Commit); err != nil {
			return fmt.Errorf("checkout %q: %w", entry.ID, err)
		}
		if entry.Git.Upstream != nil {
			if _, err := runner.Run(ctx, repo, "update-ref", entry.Git.Upstream.Ref, entry.Git.Upstream.Commit); err != nil {
				return fmt.Errorf("restore upstream ref %q: %w", entry.ID, err)
			}
			if _, err := runner.Run(ctx, repo, "branch", "--set-upstream-to="+entry.Git.Upstream.Ref, branch); err != nil {
				return fmt.Errorf("restore upstream %q: %w", entry.ID, err)
			}
		} else {
			_, _ = runner.Run(ctx, repo, "branch", "--unset-upstream", branch)
		}
		return nil
	}
	if entry.Git.Head.Commit != "" {
		if _, err := runner.Run(ctx, repo, "checkout", "--force", "--detach", entry.Git.Head.Commit); err != nil {
			return err
		}
	}
	return nil
}

func applyDelta(ctx context.Context, backup string, artifacts []ArtifactRecord, repo string, entry RepositoryEntry, runner gitinspect.Runner) error {
	for _, kind := range []string{"index-patch", "patch"} {
		path, size, ok := findArtifactPath(backup, artifacts, entry.ID, kind)
		if !ok {
			return fmt.Errorf("%s artifact missing", kind)
		}
		if size == 0 {
			continue
		}
		args := []string{"apply", "--check", "--binary", path}
		if _, err := runner.Run(ctx, repo, args...); err != nil {
			return fmt.Errorf("check %s: %w", kind, err)
		}
		if _, err := runner.Run(ctx, repo, []string{"apply", "--binary", path}...); err != nil {
			return fmt.Errorf("apply %s: %w", kind, err)
		}
		if kind == "index-patch" {
			args = []string{"apply", "--cached", "--check", "--binary", path}
			if _, err := runner.Run(ctx, repo, args...); err != nil {
				return fmt.Errorf("check cached %s: %w", kind, err)
			}
			args = []string{"apply", "--cached", "--binary", path}
			if _, err := runner.Run(ctx, repo, args...); err != nil {
				return fmt.Errorf("apply cached %s: %w", kind, err)
			}
		}
	}
	path, _, ok := findArtifactPath(backup, artifacts, entry.ID, "untracked")
	if !ok {
		return errors.New("untracked artifact missing")
	}
	return extractTar(path, repo)
}

func validateRestoredRepository(ctx context.Context, path string, entry RepositoryEntry, runner gitinspect.Runner) error {
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return fmt.Errorf("not a Git repository: %w", err)
	}
	state, err := gitinspect.InspectState(ctx, path, runner)
	if err != nil {
		return err
	}
	if entry.Git.Head.Commit != "" && state.Head.Commit != entry.Git.Head.Commit {
		return fmt.Errorf("HEAD mismatch: got %s, want %s", state.Head.Commit, entry.Git.Head.Commit)
	}
	if !reflect.DeepEqual(state, entry.Git) {
		return fmt.Errorf("git state mismatch: got %#v, want %#v", state, entry.Git)
	}
	return nil
}
