package fsfind

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitDirectoriesTreeSkipsNestedReposInsideRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	paths := []string{
		filepath.Join(root, "github.com", "example", "app", ".git"),
		filepath.Join(root, "github.com", "example", "app", ".build", "repositories", "dep1", ".git"),
		filepath.Join(root, "github.com", "example", "app", ".build", "index-build", "repositories", "dep2", ".git"),
		filepath.Join(root, "github.com", "example", "app", "vendor", "tmp", "dep3", ".git"),
		filepath.Join(root, "github.com", "example", "another", ".git"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", p, err)
		}
	}

	tree, err := GitDirectoriesTree(root)
	if err != nil {
		t.Fatalf("GitDirectoriesTree() error = %v", err)
	}

	var got []string
	for it := tree.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		got = append(got, string(node.Key()))
	}
	slices.Sort(got)

	want := []string{
		filepath.Join(root, "github.com", "example", "another"),
		filepath.Join(root, "github.com", "example", "app"),
	}
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("GitDirectoriesTree() = %v, want %v", got, want)
	}
}

func TestGitDirectoriesTreeContextStopsWhenCanceled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GitDirectoriesTreeContext(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GitDirectoriesTreeContext() error = %v, want %v", err, context.Canceled)
	}
}
