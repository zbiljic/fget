package fsfind

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitDirectoriesStrict(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoA := filepath.Join(root, "a")
	repoB := filepath.Join(root, "nested", "b")
	for _, repoPath := range []string{repoA, repoB} {
		if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := GitDirectoriesStrict(root)
	if err != nil {
		t.Fatalf("GitDirectoriesStrict() error = %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(resolvedRoot, "a"),
		filepath.Join(resolvedRoot, "nested", "b"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("GitDirectoriesStrict() = %v, want %v", got, want)
	}
}

func TestGitDirectoriesStrictResolvesSymlinkRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	repoPath := filepath.Join(realRoot, "repo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(root, "link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}

	got, err := GitDirectoriesStrict(linkRoot)
	if err != nil {
		t.Fatalf("GitDirectoriesStrict() error = %v", err)
	}
	resolvedRepoPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{resolvedRepoPath}) {
		t.Fatalf("GitDirectoriesStrict() = %v, want %v", got, []string{resolvedRepoPath})
	}
}

func TestGitDirectoriesStrictFailsOnWalkError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	wantErr := errors.New("walk failed")
	got, err := gitDirectoriesStrictWithWalk(context.Background(), func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, wantErr)
	}, root)
	if !errors.Is(err, wantErr) {
		t.Fatalf("gitDirectoriesStrictWithWalk() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("gitDirectoriesStrictWithWalk() = %v, want nil", got)
	}
}

func TestGitDirectoriesStrictRejectsBrokenRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	broken := filepath.Join(root, "broken")
	if err := os.Symlink(filepath.Join(root, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	if _, err := GitDirectoriesStrict(broken); err == nil {
		t.Fatal("GitDirectoriesStrict() error = nil, want broken symlink error")
	}
}

func TestGitDirectoriesStrictHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GitDirectoriesStrictContext(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GitDirectoriesStrictContext() error = %v, want %v", err, context.Canceled)
	}
}
