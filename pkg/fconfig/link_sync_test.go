package fconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncLinks_CreatesUpdatesRemovesAndPreservesRealPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceA := filepath.Join(t.TempDir(), "github.com", "cli", "cli")
	sourceB := filepath.Join(t.TempDir(), "github.com", "maaslalani", "nap")

	if err := os.MkdirAll(sourceA, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceA) error = %v", err)
	}
	if err := os.MkdirAll(sourceB, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceB) error = %v", err)
	}

	updatedPath := filepath.Join(root, "github.com", "maaslalani", "nap")
	if err := os.MkdirAll(filepath.Dir(updatedPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(updatedPath) error = %v", err)
	}
	if err := os.Symlink("/tmp/wrong-target", updatedPath); err != nil {
		t.Fatalf("Symlink(updatedPath) error = %v", err)
	}

	stalePath := filepath.Join(root, "github.com", "old", "repo")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(stalePath) error = %v", err)
	}
	if err := os.Symlink("/tmp/stale-target", stalePath); err != nil {
		t.Fatalf("Symlink(stalePath) error = %v", err)
	}

	conflictPath := filepath.Join(root, "github.com", "occupied", "repo")
	if err := os.MkdirAll(conflictPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(conflictPath) error = %v", err)
	}

	result, err := SyncLinks(root, []LinkTarget{
		{
			RepoID:     "github.com/cli/cli",
			SourcePath: sourceA,
			TargetPath: filepath.Join(root, "github.com", "cli", "cli"),
		},
		{
			RepoID:     "github.com/maaslalani/nap",
			SourcePath: sourceB,
			TargetPath: updatedPath,
		},
		{
			RepoID:     "github.com/occupied/repo",
			SourcePath: sourceB,
			TargetPath: conflictPath,
		},
	})
	if err == nil {
		t.Fatal("SyncLinks() error = nil, want aggregated conflict error")
	}

	if result.Created != 1 {
		t.Fatalf("SyncLinks() created = %d, want 1", result.Created)
	}
	if result.Updated != 1 {
		t.Fatalf("SyncLinks() updated = %d, want 1", result.Updated)
	}
	if result.Removed != 1 {
		t.Fatalf("SyncLinks() removed = %d, want 1", result.Removed)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("SyncLinks() skipped = %v, want 1 conflict", result.Skipped)
	}
	if result.Skipped[0].RepoID != "github.com/occupied/repo" {
		t.Fatalf("conflict repo = %q, want %q", result.Skipped[0].RepoID, "github.com/occupied/repo")
	}
	if !strings.Contains(result.Skipped[0].Err.Error(), "occupied by existing non-symlink path") {
		t.Fatalf("conflict error = %q, want non-symlink occupancy", result.Skipped[0].Err)
	}

	assertSymlinkTarget(t, filepath.Join(root, "github.com", "cli", "cli"), sourceA)
	assertSymlinkTarget(t, updatedPath, sourceB)

	if _, statErr := os.Lstat(stalePath); !os.IsNotExist(statErr) {
		t.Fatalf("stale path still exists, err = %v", statErr)
	}
}

func TestSyncLinks_LeavesCorrectSymlinkUntouched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "github.com", "acme", "api")
	target := filepath.Join(root, "github.com", "acme", "api")

	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("MkdirAll(source) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll(target parent) error = %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("Symlink(target) error = %v", err)
	}

	result, err := SyncLinks(root, []LinkTarget{{
		RepoID:     "github.com/acme/api",
		SourcePath: source,
		TargetPath: target,
	}})
	if err != nil {
		t.Fatalf("SyncLinks() error = %v", err)
	}
	if result.Created != 0 || result.Updated != 0 || result.Removed != 0 {
		t.Fatalf("SyncLinks() result = %+v, want no changes", result)
	}
}

func TestSyncLinks_RemovesEmptyParentDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stalePath := filepath.Join(root, "github.com", "old", "repo")

	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(stalePath) error = %v", err)
	}
	if err := os.Symlink("/tmp/stale-target", stalePath); err != nil {
		t.Fatalf("Symlink(stalePath) error = %v", err)
	}

	result, err := SyncLinks(root, nil)
	if err != nil {
		t.Fatalf("SyncLinks() error = %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("SyncLinks() removed = %d, want 1", result.Removed)
	}

	if _, statErr := os.Lstat(filepath.Join(root, "github.com")); !os.IsNotExist(statErr) {
		t.Fatalf("expected empty parent directories to be removed, got err = %v", statErr)
	}
}

func assertSymlinkTarget(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", path, err)
	}
	if got != want {
		t.Fatalf("Readlink(%q) = %q, want %q", path, got, want)
	}
}
