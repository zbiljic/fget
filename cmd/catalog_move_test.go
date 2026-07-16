package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestMigrateCatalogsForRepoMoveInRuntimeContext_UsesMovedRepoScope(t *testing.T) {
	t.Parallel()

	scopeRoot := t.TempDir()
	homeDir := t.TempDir()
	oldPath := filepath.Join(scopeRoot, "src", "github.com", "old-owner", "models")
	newPath := filepath.Join(scopeRoot, "src", "github.com", "new-owner", "models")
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(newPath) error = %v", err)
	}

	configPath := filepath.Join(scopeRoot, fconfigFilename)
	configContent := "version: \"2\"\n" +
		"roots:\n" +
		"  - ./src\n" +
		"catalog:\n" +
		"  path: ./fget.catalog.yaml\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath) error = %v", err)
	}

	now := time.Now().UTC()
	catalogPath := filepath.Join(scopeRoot, "fget.catalog.yaml")
	catalog := &fconfig.Catalog{
		Version:   fconfig.CatalogVersionV1,
		ScopeRoot: scopeRoot,
		Repos: []fconfig.RepoEntry{
			{
				ID:        "github.com/old-owner/models",
				RemoteURL: "https://github.com/old-owner/models",
				Tags:      []string{"w:zucli"},
				Locations: []fconfig.RepoLocation{
					{Path: oldPath, LastSeenAt: now.Add(-time.Hour)},
				},
			},
			{
				ID:        "github.com/new-owner/models",
				RemoteURL: "https://github.com/new-owner/models",
				Tags:      []string{"models"},
				Locations: []fconfig.RepoLocation{
					{Path: newPath, LastSeenAt: now.Add(-time.Minute)},
				},
			},
		},
	}
	if err := fconfig.SaveCatalog(catalogPath, catalog); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}

	err := migrateCatalogsForRepoMoveInRuntimeContext(fconfig.RepoMove{
		OldID:   "github.com/old-owner/models",
		NewID:   "github.com/new-owner/models",
		OldURL:  "https://github.com/old-owner/models",
		NewURL:  "https://github.com/new-owner/models",
		OldPath: oldPath,
		NewPath: newPath,
		MovedAt: now,
	}, configRuntimeContext{
		HomeDir: homeDir,
		Cwd:     homeDir,
	})
	if err != nil {
		t.Fatalf("migrateCatalogsForRepoMoveInRuntimeContext() error = %v", err)
	}

	loaded, err := fconfig.LoadCatalogWithScope(catalogPath, scopeRoot)
	if err != nil {
		t.Fatalf("LoadCatalogWithScope() error = %v", err)
	}
	if len(loaded.Repos) != 1 {
		t.Fatalf("loaded repo count = %d, want 1", len(loaded.Repos))
	}
	repo := loaded.Repos[0]
	if repo.ID != "github.com/new-owner/models" {
		t.Fatalf("loaded repo id = %q, want canonical id", repo.ID)
	}
	if !reflect.DeepEqual(repo.Tags, []string{"models", "w:zucli"}) {
		t.Fatalf("loaded repo tags = %v, want merged tags", repo.Tags)
	}
	if len(repo.Locations) != 1 || repo.Locations[0].Path != newPath {
		t.Fatalf("loaded repo locations = %#v, want moved path", repo.Locations)
	}

	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("ReadFile(catalogPath) error = %v", err)
	}
	wantRelativePath := "path: src/github.com/new-owner/models"
	if !strings.Contains(string(raw), wantRelativePath) {
		t.Fatalf("saved catalog should preserve scope-relative path %q, got:\n%s", wantRelativePath, raw)
	}
}

func TestMigrateCatalogsForRepoMoveInRuntimeContext_MissingCatalogIsNoop(t *testing.T) {
	t.Parallel()

	scopeRoot := t.TempDir()
	homeDir := t.TempDir()
	newPath := filepath.Join(scopeRoot, "src", "github.com", "new-owner", "models")
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(newPath) error = %v", err)
	}

	err := migrateCatalogsForRepoMoveInRuntimeContext(fconfig.RepoMove{
		OldID:   "github.com/old-owner/models",
		NewID:   "github.com/new-owner/models",
		OldPath: filepath.Join(scopeRoot, "src", "github.com", "old-owner", "models"),
		NewPath: newPath,
	}, configRuntimeContext{
		HomeDir: homeDir,
		Cwd:     scopeRoot,
	})
	if err != nil {
		t.Fatalf("migrateCatalogsForRepoMoveInRuntimeContext() error = %v, want nil", err)
	}
}

func TestGitMove_ExistingDestinationReturnsCatalogMove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldPath := filepath.Join(root, "github.com", "old-owner", "models")
	newPath := filepath.Join(root, "github.com", "new-owner", "models")
	for _, path := range []string{oldPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
	}

	move, err := gitMove(
		context.Background(),
		oldPath,
		"https://github.com/old-owner/models",
		"https://github.com/new-owner/models",
	)
	if err != nil {
		t.Fatalf("gitMove() error = %v", err)
	}
	if move == nil {
		t.Fatal("gitMove() move = nil, want catalog move")
	}
	if move.OldPath != oldPath || move.NewPath != newPath {
		t.Fatalf("gitMove() paths = %q -> %q, want %q -> %q", move.OldPath, move.NewPath, oldPath, newPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists after gitMove(), err = %v", err)
	}
}

func TestGitMove_RenamesRepoUpdatesRemoteAndReturnsCatalogMove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldURL := "https://github.com/old-owner/models"
	newURL := "https://github.com/new-owner/models"
	oldPath := filepath.Join(root, "github.com", "old-owner", "models")
	newPath := filepath.Join(root, "github.com", "new-owner", "models")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(oldPath) error = %v", err)
	}

	repo, err := git.PlainInit(oldPath, false)
	if err != nil {
		t.Fatalf("PlainInit(oldPath) error = %v", err)
	}
	if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{oldURL},
	}); err != nil {
		t.Fatalf("CreateRemote(origin) error = %v", err)
	}

	move, err := gitMove(context.Background(), oldPath, oldURL, newURL)
	if err != nil {
		t.Fatalf("gitMove() error = %v", err)
	}
	if move == nil {
		t.Fatal("gitMove() move = nil, want catalog move")
	}
	if move.OldID != "github.com/old-owner/models" || move.NewID != "github.com/new-owner/models" {
		t.Fatalf("gitMove() ids = %q -> %q, want old and new ids", move.OldID, move.NewID)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists after gitMove(), err = %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path does not exist after gitMove(): %v", err)
	}

	remoteURL, err := gitRemoteConfigURL(newPath)
	if err != nil {
		t.Fatalf("gitRemoteConfigURL(newPath) error = %v", err)
	}
	if remoteURL.String() != newURL {
		t.Fatalf("new origin URL = %q, want %q", remoteURL.String(), newURL)
	}
}
