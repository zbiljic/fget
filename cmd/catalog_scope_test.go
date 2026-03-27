package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func TestLoadCatalogSetForEffectiveConfig_LoadsImportedCatalogs(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	scopeDir := filepath.Join(homeDir, "scope")
	importedScopeDir := filepath.Join(homeDir, "external")

	if err := mkdirAll(scopeDir, importedScopeDir); err != nil {
		t.Fatalf("mkdirAll() error = %v", err)
	}

	ownedCatalogPath := filepath.Join(scopeDir, "fget.catalog.yaml")
	importedCatalogPath := filepath.Join(importedScopeDir, "fget.catalog.yaml")
	importedConfigPath := filepath.Join(importedScopeDir, "fget.yaml")

	now := time.Now().UTC()
	if err := fconfig.SaveCatalog(ownedCatalogPath, &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Repos: []fconfig.RepoEntry{
			{
				ID:   "github.com/acme/api",
				Tags: []string{"home"},
				Locations: []fconfig.RepoLocation{
					{Path: "/repos/home/api", LastSeenAt: now},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveCatalog(owned) error = %v", err)
	}
	if err := fconfig.SaveCatalog(importedCatalogPath, &fconfig.Catalog{
		Version: fconfig.CatalogVersionV1,
		Repos: []fconfig.RepoEntry{
			{
				ID:   "github.com/acme/api",
				Tags: []string{"ssd"},
				Locations: []fconfig.RepoLocation{
					{Path: "/repos/ssd/api", LastSeenAt: now.Add(time.Minute)},
				},
			},
			{
				ID:   "github.com/acme/worker",
				Tags: []string{"external"},
				Locations: []fconfig.RepoLocation{
					{Path: "/repos/ssd/worker", LastSeenAt: now},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveCatalog(imported) error = %v", err)
	}

	importedConfig := "" +
		"version: \"2\"\n" +
		"catalog:\n" +
		"  path: ./fget.catalog.yaml\n"
	if err := writeFile(importedConfigPath, importedConfig); err != nil {
		t.Fatalf("writeFile(importedConfigPath) error = %v", err)
	}

	set, err := loadCatalogSetForEffectiveConfig(&fconfig.EffectiveConfig{
		Config: fconfig.Config{
			Version: fconfig.ConfigVersionV2,
			Catalog: fconfig.CatalogConfig{
				Path:    ownedCatalogPath,
				Imports: []string{importedConfigPath},
			},
		},
		ScopeOwner: filepath.Join(scopeDir, "fget.yaml"),
	}, homeDir)
	if err != nil {
		t.Fatalf("loadCatalogSetForEffectiveConfig() error = %v", err)
	}

	if len(set.Sources) != 2 {
		t.Fatalf("source count = %d, want 2", len(set.Sources))
	}

	index, err := fconfig.ResolveRepoIndex(set.View, "github.com/acme/api")
	if err != nil {
		t.Fatalf("ResolveRepoIndex() error = %v", err)
	}

	repo := set.View.Repos[index]
	if !reflect.DeepEqual(repo.Tags, []string{"home", "ssd"}) {
		t.Fatalf("merged tags = %v, want %v", repo.Tags, []string{"home", "ssd"})
	}
	if len(repo.Locations) != 2 {
		t.Fatalf("merged locations = %d, want 2", len(repo.Locations))
	}

	sources, err := set.resolveTagSources("github.com/acme/api")
	if err != nil {
		t.Fatalf("resolveTagSources() error = %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("source count for shared repo = %d, want 2", len(sources))
	}
	if sources[0].CatalogPath != ownedCatalogPath {
		t.Fatalf("first shared source path = %q, want %q", sources[0].CatalogPath, ownedCatalogPath)
	}
	if sources[1].CatalogPath != importedCatalogPath {
		t.Fatalf("second shared source path = %q, want %q", sources[1].CatalogPath, importedCatalogPath)
	}

	sources, err = set.resolveTagSources("github.com/acme/worker")
	if err != nil {
		t.Fatalf("resolveTagSources(imported) error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count for imported repo = %d, want 1", len(sources))
	}
	if sources[0].CatalogPath != importedCatalogPath {
		t.Fatalf("imported catalog path = %q, want %q", sources[0].CatalogPath, importedCatalogPath)
	}
}

func mkdirAll(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
