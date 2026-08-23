package fbackup

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zbiljic/fget/pkg/gitinspect"
)

//nolint:gocyclo // this fixture intentionally exercises every delta layer and ref class.
func TestRestoreDeltaLayers(t *testing.T) {
	repository := newRecloneableRepository(t, "delta/repo")
	baseRef := repository.Git.Head.Ref
	mustGitTest(t, repository.Path, "checkout", "-qb", "local-only")
	if err := os.WriteFile(filepath.Join(repository.Path, "branch-only"), []byte("branch"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repository.Path, "add", "branch-only")
	mustGitTest(t, repository.Path, "commit", "-qm", "local branch")
	mustGitTest(t, repository.Path, "tag", "local-tag")
	mustGitTest(t, repository.Path, "checkout", "-q", baseRef)
	if err := os.WriteFile(filepath.Join(repository.Path, "tracked"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repository.Path, "add", "tracked")
	mustGitTest(t, repository.Path, "commit", "-qm", "base tracked")
	state, err := gitinspect.InspectState(context.Background(), repository.Path, gitinspect.CLIRunner{})
	if err != nil {
		t.Fatal(err)
	}
	repository.Git = state
	if err := os.WriteFile(filepath.Join(repository.Path, "staged"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitTest(t, repository.Path, "add", "staged")
	if err := os.WriteFile(filepath.Join(repository.Path, "tracked"), []byte("working"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository.Path, "untracked"), []byte("untracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tracked", filepath.Join(repository.Path, "untracked-link")); err != nil {
		t.Fatal(err)
	}
	repository.Classification = ClassificationDelta
	backup := filepath.Join(t.TempDir(), "backup")
	if err := Create(context.Background(), CreateOptions{Destination: backup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{repository}}}); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	report, err := RestoreWithReport(context.Background(), RestoreOptions{Backup: backup, Destination: restored})
	if err != nil {
		t.Fatalf("%v report=%+v", err, report)
	}
	for name, want := range map[string]string{"staged": "staged", "tracked": "working", "untracked": "untracked"} {
		data, err := os.ReadFile(filepath.Join(restored, "delta", "repo", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}
	untrackedInfo, err := os.Stat(filepath.Join(restored, "delta", "repo", "untracked"))
	if err != nil {
		t.Fatal(err)
	}
	if untrackedInfo.Mode().Perm() != 0o600 {
		t.Fatalf("untracked mode = %o", untrackedInfo.Mode().Perm())
	}
	linkInfo, err := os.Lstat(filepath.Join(restored, "delta", "repo", "untracked-link"))
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("untracked-link was not restored as a symlink")
	}
	if target, err := os.Readlink(filepath.Join(restored, "delta", "repo", "untracked-link")); err != nil || target != "tracked" {
		t.Fatalf("symlink target = %q, err=%v", target, err)
	}
	status, err := gitinspect.CLIRunner{}.Run(context.Background(), filepath.Join(restored, "delta", "repo"), "status", "--porcelain=v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Stdout, "A  staged") || !strings.Contains(status.Stdout, " M tracked") {
		t.Fatalf("restored status = %q", status.Stdout)
	}
	restoredState, err := gitinspect.InspectState(context.Background(), filepath.Join(restored, "delta", "repo"), gitinspect.CLIRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if restoredState.LocalRefsDigest != repository.Git.LocalRefsDigest || restoredState.LocalRefCount != repository.Git.LocalRefCount {
		t.Fatalf("restored refs = %#v, want %#v", restoredState, repository.Git)
	}
	if restoredState.Head.Ref != repository.Git.Head.Ref || !reflect.DeepEqual(restoredState.Upstream, repository.Git.Upstream) {
		t.Fatalf("restored symbolic/upstream state = %#v, want %#v", restoredState, repository.Git)
	}
	if _, err := os.Stat(filepath.Join(restored, "delta", "repo", ".git", "refs", "heads", "local-only")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restored, "delta", "repo", ".git", "refs", "tags", "local-tag")); err != nil {
		t.Fatal(err)
	}
	fallbackRepository := repository
	fallbackRepository.RemoteURL = filepath.Join(t.TempDir(), "missing.git")
	fallbackBackup := filepath.Join(t.TempDir(), "fallback-backup")
	if err := Create(context.Background(), CreateOptions{Destination: fallbackBackup, Manifest: Manifest{Version: ManifestVersion, Repositories: []RepositoryEntry{fallbackRepository}}}); err != nil {
		t.Fatal(err)
	}
	fallbackDestination := filepath.Join(t.TempDir(), "fallback-destination")
	if err := Restore(context.Background(), RestoreOptions{Backup: fallbackBackup, Destination: fallbackDestination}); err != nil {
		t.Fatalf("bundle fallback failed: %v", err)
	}
	fallbackState, err := gitinspect.InspectState(context.Background(), filepath.Join(fallbackDestination, "delta", "repo"), gitinspect.CLIRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fallbackState, repository.Git) {
		t.Fatalf("fallback Git state = %#v, want %#v", fallbackState, repository.Git)
	}
}
