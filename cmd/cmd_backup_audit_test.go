package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zbiljic/fget/pkg/fbackup"
	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestBackupAuditDeterministicJSON(t *testing.T) {
	tmpDir := t.TempDir()
	rootA := filepath.Join(tmpDir, "root-b")
	rootB := filepath.Join(tmpDir, "root-a")
	mustMkdirAll(t, filepath.Join(rootA, "repo-a", ".git"))
	mustMkdirAll(t, filepath.Join(rootB, "repo-b", ".git"))

	originalFind := backupAuditFindReposFn
	originalInspect := backupAuditInspectRepoFn
	originalNow := backupAuditNowFn
	originalFlags := backupAuditCmdFlags
	t.Cleanup(func() {
		backupAuditFindReposFn = originalFind
		backupAuditInspectRepoFn = originalInspect
		backupAuditNowFn = originalNow
		backupAuditCmdFlags = originalFlags
	})

	backupAuditFindReposFn = func(context.Context, ...string) ([]string, error) {
		return []string{
			filepath.Join(rootA, "repo-a"),
			filepath.Join(rootB, "repo-b"),
		}, nil
	}
	backupAuditInspectRepoFn = func(_ context.Context, repoPath string, _ backupAuditFlags, _ *backupCatalogIndex) (fbackup.RepositoryEntry, error) {
		base := filepath.Base(repoPath)
		return fbackup.RepositoryEntry{
			ID:             "github.com/acme/" + base,
			Path:           repoPath,
			Classification: fbackup.ClassificationUnknown,
			ReasonCodes:    []string{"remote-unchecked"},
			RemoteState:    fbackup.RemoteStateUnchecked,
		}, nil
	}
	backupAuditCmdFlags = backupAuditFlags{
		Output:  "-",
		Workers: 2,
	}

	runAudit := func(at time.Time) string {
		backupAuditNowFn = func() time.Time { return at }
		command := &cobra.Command{}
		command.SetContext(context.Background())
		var stdout bytes.Buffer
		command.SetOut(&stdout)

		if err := runBackupAudit(command, []string{rootA, rootB}); err != nil {
			t.Fatalf("runBackupAudit() error = %v", err)
		}
		return normalizeGeneratedAt(t, stdout.Bytes())
	}

	first := runAudit(time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC))
	second := runAudit(time.Date(2026, time.August, 20, 12, 0, 3, 0, time.UTC))
	if first != second {
		t.Fatalf("normalized manifests differ:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestBackupAuditRejectsOutputInsideRepository(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	mustMkdirAll(t, filepath.Join(repoPath, ".git"))

	err := ensureBackupAuditOutputOutsideRepos(filepath.Join(repoPath, "backup.json"), []string{repoPath})
	if err == nil {
		t.Fatal("ensureBackupAuditOutputOutsideRepos() error = nil, want containment error")
	}
}

func TestWriteBackupAuditFilePreservesDestinationOnWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeErr := errors.New("write failed")
	err := writeBackupAuditFile(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial\n"); err != nil {
			return err
		}
		return writeErr
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("writeBackupAuditFile() error = %v, want %v", err, writeErr)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old\n" {
		t.Fatalf("content = %q, want old", content)
	}
}

func normalizeGeneratedAt(t *testing.T, manifestJSON []byte) string {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(manifestJSON, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	payload["generated_at"] = "normalized"
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	return string(encoded)
}

func TestBackupAuditCatalogMissingIsOptional(t *testing.T) {
	ctx := configRuntimeContext{
		HomeDir:       t.TempDir(),
		Cwd:           t.TempDir(),
		XDGConfigHome: "",
	}
	config := &fconfig.EffectiveConfig{}

	catalog, err := loadBackupAuditCatalog(config, ctx, "")
	if err != nil {
		t.Fatalf("loadBackupAuditCatalog() error = %v", err)
	}
	if catalog != nil {
		t.Fatalf("loadBackupAuditCatalog() = %+v, want nil", catalog)
	}
}

func TestBackupAuditManifestWriteIncludesSanitizedRemoteURL(t *testing.T) {
	var stdout bytes.Buffer
	manifest := fbackup.Manifest{
		Version:     fbackup.ManifestVersion,
		GeneratedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
		Roots:       []string{"/src"},
		Repositories: []fbackup.RepositoryEntry{
			fbackup.BuildRepositoryEntry(fbackup.RepositoryProbe{
				ID:          "github.com/acme/repo",
				Path:        "/src/repo",
				RemoteURL:   "https://user:secret@example.com/acme/repo.git",
				RemoteState: fbackup.RemoteStateUnchecked,
			}),
		},
	}

	if err := writeBackupAuditManifest(&stdout, manifest); err != nil {
		t.Fatalf("writeBackupAuditManifest() error = %v", err)
	}
	if strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "user@") {
		t.Fatalf("manifest contains unsanitized remote URL:\n%s", stdout.String())
	}
}

func TestBackupAuditManifestDropsMalformedCredentialBearingRemoteURL(t *testing.T) {
	var stdout bytes.Buffer
	manifest := fbackup.Manifest{
		Version:     fbackup.ManifestVersion,
		GeneratedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
		Roots:       []string{"/src"},
		Repositories: []fbackup.RepositoryEntry{
			fbackup.BuildRepositoryEntry(fbackup.RepositoryProbe{
				ID:          "github.com/acme/repo",
				Path:        "/src/repo",
				RemoteURL:   "https://user:secret%zz@example.com/acme/repo.git",
				RemoteState: fbackup.RemoteStateUnchecked,
			}),
		},
	}

	if err := writeBackupAuditManifest(&stdout, manifest); err != nil {
		t.Fatalf("writeBackupAuditManifest() error = %v", err)
	}
	if strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "user:secret%zz@") || strings.Contains(stdout.String(), "user@") {
		t.Fatalf("manifest contains malformed credential-bearing remote URL:\n%s", stdout.String())
	}
}

func TestBackupAuditLocalOnlyBranchPreventsRecloneable(t *testing.T) {
	repoDir, remoteDir := initCommittedRepo(t)
	configureOrigin(t, repoDir, remoteDir)
	pushMain(t, repoDir)
	gitRun(t, repoDir, "checkout", "-b", "feature/local-only")
	writeTestFile(t, filepath.Join(repoDir, "branch-only.txt"), "branch-only\n")
	gitRun(t, repoDir, "add", "branch-only.txt")
	gitRun(t, repoDir, "commit", "-m", "branch only")
	gitRun(t, repoDir, "checkout", "main")

	record, err := inspectBackupAuditRepository(context.Background(), repoDir, backupAuditFlags{
		VerifyRemotes: true,
		Workers:       1,
	}, nil)
	if err != nil {
		t.Fatalf("inspectBackupAuditRepository() error = %v", err)
	}
	if record.LocalOnlyCommitCount != 1 {
		t.Fatalf("LocalOnlyCommitCount = %d, want 1", record.LocalOnlyCommitCount)
	}
	if record.Git.Head.Commit == "" || record.Git.Head.Ref != "refs/heads/main" {
		t.Fatalf("Git.Head = %+v, want attached main commit", record.Git.Head)
	}
	if record.Git.Upstream == nil || record.Git.Upstream.Ref != "refs/remotes/origin/main" {
		t.Fatalf("Git.Upstream = %+v, want origin/main", record.Git.Upstream)
	}
	if record.Git.LocalRefCount != 2 || record.Git.LocalRefsDigest == "" {
		t.Fatalf("Git local refs = %d, %q, want two refs and digest", record.Git.LocalRefCount, record.Git.LocalRefsDigest)
	}
	if record.Classification == fbackup.ClassificationRecloneable {
		t.Fatalf("Classification = %q, want non-recloneable", record.Classification)
	}
}

func TestCollectBackupAuditRecordsHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	records, err := collectBackupAuditRecords(ctx, []string{"/repo"}, backupAuditFlags{Workers: 1}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("collectBackupAuditRecords() error = %v, want %v", err, context.Canceled)
	}
	if records != nil {
		t.Fatalf("records = %v, want nil", records)
	}
}

func TestCollectBackupAuditRecordsHonorsDeadlineExceeded(t *testing.T) {
	originalInspect := backupAuditInspectRepoFn
	t.Cleanup(func() {
		backupAuditInspectRepoFn = originalInspect
	})
	backupAuditInspectRepoFn = func(_ context.Context, repoPath string, _ backupAuditFlags, _ *backupCatalogIndex) (fbackup.RepositoryEntry, error) {
		return fbackup.RepositoryEntry{
			ID:             repoPath,
			Path:           repoPath,
			Classification: fbackup.ClassificationUnknown,
			RemoteState:    fbackup.RemoteStateUnchecked,
		}, nil
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	records, err := collectBackupAuditRecords(ctx, []string{"/repo"}, backupAuditFlags{Workers: 1}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("collectBackupAuditRecords() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if records != nil {
		t.Fatalf("records = %v, want nil", records)
	}
}

func TestRunBackupAuditDiscoveryErrorLeavesNoFinalManifest(t *testing.T) {
	originalFind := backupAuditFindReposFn
	originalFlags := backupAuditCmdFlags
	t.Cleanup(func() {
		backupAuditFindReposFn = originalFind
		backupAuditCmdFlags = originalFlags
	})

	root := t.TempDir()
	outputPath := filepath.Join(root, "manifest.json")
	discoveryErr := errors.New("discovery failed")
	backupAuditFindReposFn = func(context.Context, ...string) ([]string, error) {
		return nil, discoveryErr
	}
	backupAuditCmdFlags = backupAuditFlags{
		Output:  outputPath,
		Workers: 1,
	}

	command := &cobra.Command{}
	command.SetContext(context.Background())

	err := runBackupAudit(command, []string{root})
	if !errors.Is(err, discoveryErr) {
		t.Fatalf("runBackupAudit() error = %v, want %v", err, discoveryErr)
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want %v", outputPath, statErr, os.ErrNotExist)
	}
}

func TestBackupAuditSubmoduleWithHiddenBranchIsProblem(t *testing.T) {
	rootDir := t.TempDir()
	superRepoDir := filepath.Join(rootDir, "super")
	subRepoDir := filepath.Join(rootDir, "sub-work")
	superRemoteDir := filepath.Join(rootDir, "super-remote.git")
	subRemoteDir := filepath.Join(rootDir, "sub-remote.git")

	initRepoAtPath(t, superRepoDir, superRemoteDir)
	configureOrigin(t, superRepoDir, superRemoteDir)
	pushMain(t, superRepoDir)

	initRepoAtPath(t, subRepoDir, subRemoteDir)
	configureOrigin(t, subRepoDir, subRemoteDir)
	pushMain(t, subRepoDir)

	gitRunWithEnv(t, superRepoDir, []string{"GIT_ALLOW_PROTOCOL=file"}, "submodule", "add", subRemoteDir, "deps/sub")
	gitRun(t, superRepoDir, "commit", "-am", "add submodule")
	gitRun(t, superRepoDir, "push", "origin", "main")

	submodulePath := filepath.Join(superRepoDir, "deps", "sub")
	gitRun(t, submodulePath, "config", "user.name", "Backup Audit")
	gitRun(t, submodulePath, "config", "user.email", "backup@example.com")
	gitRun(t, submodulePath, "checkout", "-b", "hidden/local-only")
	writeTestFile(t, filepath.Join(submodulePath, "hidden.txt"), "hidden\n")
	gitRun(t, submodulePath, "add", "hidden.txt")
	gitRun(t, submodulePath, "commit", "-m", "hidden branch commit")
	gitRun(t, submodulePath, "checkout", "main")

	if status := strings.TrimSpace(gitOutput(t, superRepoDir, "status", "--porcelain")); status != "" {
		t.Fatalf("superproject status = %q, want clean", status)
	}

	originalFlags := backupAuditCmdFlags
	t.Cleanup(func() {
		backupAuditCmdFlags = originalFlags
	})
	backupAuditCmdFlags = backupAuditFlags{
		Output:        "-",
		VerifyRemotes: true,
		Workers:       1,
	}

	command := &cobra.Command{}
	command.SetContext(context.Background())
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := runBackupAudit(command, []string{superRepoDir}); err != nil {
		t.Fatalf("runBackupAudit() error = %v", err)
	}

	var manifest fbackup.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(manifest.Repositories) != 1 {
		t.Fatalf("len(manifest.Repositories) = %d, want 1", len(manifest.Repositories))
	}
	record := manifest.Repositories[0]
	if !record.HasSubmodules {
		t.Fatalf("HasSubmodules = false, want true")
	}
	if record.Classification != fbackup.ClassificationProblem {
		t.Fatalf("Classification = %q, want %q", record.Classification, fbackup.ClassificationProblem)
	}
}

func TestBackupAuditManifestRemoteReasonRedacted(t *testing.T) {
	state, reason := verifyBackupRemote(context.Background(), "/repo", backupGitStub{
		result: backupGitOutput{},
		err: &backupGitCommandError{
			Stderr:   "fatal: Authentication failed for 'https://user:token@example.com/acme/repo.git'",
			ExitCode: 128,
			Err:      errors.New("exit status 128"),
		},
	})

	var stdout bytes.Buffer
	manifest := fbackup.Manifest{
		Version:     fbackup.ManifestVersion,
		GeneratedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
		Roots:       []string{"/src"},
		Repositories: []fbackup.RepositoryEntry{
			fbackup.BuildRepositoryEntry(fbackup.RepositoryProbe{
				ID:           "github.com/acme/repo",
				Path:         "/src/repo",
				RemoteState:  state,
				RemoteReason: reason,
			}),
		},
	}

	if err := writeBackupAuditManifest(&stdout, manifest); err != nil {
		t.Fatalf("writeBackupAuditManifest() error = %v", err)
	}
	output := stdout.String()
	if strings.Contains(output, "token") || strings.Contains(output, "user:") || strings.Contains(output, "example.com/acme/repo.git") {
		t.Fatalf("manifest contains unredacted remote reason:\n%s", output)
	}
	if !strings.Contains(output, "remote authentication failed") {
		t.Fatalf("manifest = %q, want generic remote authentication failure reason", output)
	}
}

func TestBackupAuditSymlinkRootEnumeratesResolvedRepository(t *testing.T) {
	rootDir := t.TempDir()
	targetRoot := filepath.Join(rootDir, "target-root")
	symlinkRoot := filepath.Join(rootDir, "symlink-root")
	repoDir := filepath.Join(targetRoot, "repo")
	outputPath := filepath.Join(rootDir, "manifest.json")

	mustMkdirAll(t, filepath.Join(repoDir, ".git"))
	if err := os.Symlink(targetRoot, symlinkRoot); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	originalFlags := backupAuditCmdFlags
	originalInspect := backupAuditInspectRepoFn
	t.Cleanup(func() {
		backupAuditCmdFlags = originalFlags
		backupAuditInspectRepoFn = originalInspect
	})
	backupAuditCmdFlags = backupAuditFlags{
		Output:  outputPath,
		Workers: 1,
	}
	backupAuditInspectRepoFn = func(_ context.Context, repoPath string, _ backupAuditFlags, _ *backupCatalogIndex) (fbackup.RepositoryEntry, error) {
		return fbackup.RepositoryEntry{
			ID:             "github.com/acme/repo",
			Path:           repoPath,
			Classification: fbackup.ClassificationUnknown,
			ReasonCodes:    []string{"remote-unchecked"},
			RemoteState:    fbackup.RemoteStateUnchecked,
		}, nil
	}

	command := &cobra.Command{}
	command.SetContext(context.Background())
	if err := runBackupAudit(command, []string{symlinkRoot}); err != nil {
		t.Fatalf("runBackupAudit() error = %v", err)
	}

	manifestBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", outputPath, err)
	}

	var manifest fbackup.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(manifest.Repositories) != 1 {
		t.Fatalf("len(manifest.Repositories) = %d, want 1", len(manifest.Repositories))
	}
	if got := manifest.Roots; len(got) != 1 || got[0] != filepath.Clean(symlinkRoot) {
		t.Fatalf("manifest.Roots = %v, want [%s]", got, filepath.Clean(symlinkRoot))
	}
	wantRepoPath, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", repoDir, err)
	}
	if got := manifest.Repositories[0].Path; got != wantRepoPath {
		t.Fatalf("repository path = %q, want %q", got, wantRepoPath)
	}
}

func TestBackupAuditBrokenSymlinkRootFailsWithoutManifest(t *testing.T) {
	rootDir := t.TempDir()
	brokenTarget := filepath.Join(rootDir, "missing-target")
	symlinkRoot := filepath.Join(rootDir, "broken-root")
	outputPath := filepath.Join(rootDir, "manifest.json")
	if err := os.Symlink(brokenTarget, symlinkRoot); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	originalFlags := backupAuditCmdFlags
	t.Cleanup(func() {
		backupAuditCmdFlags = originalFlags
	})
	backupAuditCmdFlags = backupAuditFlags{
		Output:  outputPath,
		Workers: 1,
	}

	command := &cobra.Command{}
	command.SetContext(context.Background())
	err := runBackupAudit(command, []string{symlinkRoot})
	if err == nil {
		t.Fatal("runBackupAudit() error = nil, want broken symlink error")
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want %v", outputPath, statErr, os.ErrNotExist)
	}
}

func TestBackupAuditRejectsOutputInsideResolvedTargetWhenRootIsSymlink(t *testing.T) {
	rootDir := t.TempDir()
	targetRoot := filepath.Join(rootDir, "target-root")
	symlinkRoot := filepath.Join(rootDir, "symlink-root")
	repoDir := filepath.Join(targetRoot, "repo")
	outputPath := filepath.Join(repoDir, "manifest.json")

	mustMkdirAll(t, filepath.Join(repoDir, ".git"))
	if err := os.Symlink(targetRoot, symlinkRoot); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	originalFlags := backupAuditCmdFlags
	originalInspect := backupAuditInspectRepoFn
	t.Cleanup(func() {
		backupAuditCmdFlags = originalFlags
		backupAuditInspectRepoFn = originalInspect
	})
	backupAuditCmdFlags = backupAuditFlags{
		Output:  outputPath,
		Workers: 1,
	}
	backupAuditInspectRepoFn = func(_ context.Context, repoPath string, _ backupAuditFlags, _ *backupCatalogIndex) (fbackup.RepositoryEntry, error) {
		return fbackup.RepositoryEntry{
			ID:             "github.com/acme/repo",
			Path:           repoPath,
			Classification: fbackup.ClassificationUnknown,
			ReasonCodes:    []string{"remote-unchecked"},
			RemoteState:    fbackup.RemoteStateUnchecked,
		}, nil
	}

	command := &cobra.Command{}
	command.SetContext(context.Background())
	err := runBackupAudit(command, []string{symlinkRoot})
	if err == nil {
		t.Fatal("runBackupAudit() error = nil, want containment error")
	}
	if !strings.Contains(err.Error(), "must be outside audited repository") {
		t.Fatalf("runBackupAudit() error = %v, want containment message", err)
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want %v", outputPath, statErr, os.ErrNotExist)
	}
}

func initRepoAtPath(t *testing.T, repoDir, remoteDir string) {
	t.Helper()
	mustMkdirAll(t, repoDir)
	baseDir := filepath.Dir(repoDir)
	gitRun(t, repoDir, "init")
	gitRun(t, repoDir, "config", "user.name", "Backup Audit")
	gitRun(t, repoDir, "config", "user.email", "backup@example.com")
	writeTestFile(t, filepath.Join(repoDir, "tracked.txt"), fmt.Sprintf("%s\n", filepath.Base(repoDir)))
	gitRun(t, repoDir, "add", "tracked.txt")
	gitRun(t, repoDir, "commit", "-m", "initial")
	gitRun(t, baseDir, "init", "--bare", "--initial-branch=main", remoteDir)
}

func gitRunWithEnv(t *testing.T, dir string, extraEnv []string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(append([]string{}, os.Environ()...), extraEnv...)
	command.Env = append(command.Env, "GIT_OPTIONAL_LOCKS=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
