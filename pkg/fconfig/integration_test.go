package fconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEndToEnd_LoadMergeSyncTagFlow(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	cwd := filepath.Join(homeDir, "dev", "proj")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}

	homeConfigPath := filepath.Join(homeDir, "fget.yaml")
	if err := os.WriteFile(homeConfigPath, []byte("version: \"1\"\nroots:\n  - ~/dev\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(homeConfigPath) error = %v", err)
	}

	projectConfigPath := filepath.Join(homeDir, "dev", "fget.yaml")
	if err := os.WriteFile(projectConfigPath, []byte("version: \"1\"\ncatalog:\n  path: \"~/custom/catalog.yaml\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(projectConfigPath) error = %v", err)
	}

	effectiveConfig, err := LoadEffectiveConfig(homeDir, cwd, "")
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	catalog, err := LoadCatalog(effectiveConfig.CatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	now := time.Now().UTC()
	find := func(roots ...string) ([]string, error) {
		return []string{"/repos/acme-api"}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		return RepoMetadata{
			ID:        "github.com/acme/api",
			Path:      path,
			RemoteURL: "https://github.com/acme/api",
		}, nil
	}

	if err := SyncCatalog(catalog, SyncOptions{Roots: effectiveConfig.Roots, Prune: true}, find, inspect, now); err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	if err := AddTags(catalog, "github.com/acme/api", []string{"alpha", "beta"}); err != nil {
		t.Fatalf("AddTags() error = %v", err)
	}
	if err := RemoveTags(catalog, "github.com/acme/api", []string{"beta"}); err != nil {
		t.Fatalf("RemoveTags() error = %v", err)
	}

	if err := SaveCatalog(effectiveConfig.CatalogPath, catalog); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}

	loadedCatalog, err := LoadCatalog(effectiveConfig.CatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog() after save error = %v", err)
	}

	if len(loadedCatalog.Repos) != 1 {
		t.Fatalf("loaded catalog repo count = %d, want 1", len(loadedCatalog.Repos))
	}
	if loadedCatalog.Repos[0].ID != "github.com/acme/api" {
		t.Fatalf("loaded repo id = %q, want %q", loadedCatalog.Repos[0].ID, "github.com/acme/api")
	}

	wantTags := []string{"alpha"}
	if !reflect.DeepEqual(loadedCatalog.Repos[0].Tags, wantTags) {
		t.Fatalf("loaded repo tags = %v, want %v", loadedCatalog.Repos[0].Tags, wantTags)
	}
	if len(loadedCatalog.Repos[0].Locations) != 1 {
		t.Fatalf("loaded repo locations = %d, want 1", len(loadedCatalog.Repos[0].Locations))
	}
}
