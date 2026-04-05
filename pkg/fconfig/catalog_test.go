package fconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCatalogUpsert_PreservesExistingTagsAndLocations(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	catalog := &Catalog{
		Version: CatalogVersionV1,
		Roots: []CatalogRoot{
			{Path: "/repos", LastScannedAt: now.Add(-time.Hour)},
		},
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/api",
				RemoteURL: "https://github.com/acme/api",
				Tags:      []string{"backend", "critical"},
				Locations: []RepoLocation{
					{Path: "/repos/api", LastSeenAt: now.Add(-time.Hour)},
				},
			},
		},
	}

	catalog.Upsert(RepoEntry{
		ID:        "github.com/acme/api",
		RemoteURL: "https://github.com/acme/new-api",
		Locations: []RepoLocation{
			{Path: "/repos/new-api", LastSeenAt: now},
		},
	})

	if len(catalog.Repos) != 1 {
		t.Fatalf("catalog repo count = %d, want 1", len(catalog.Repos))
	}

	repo := catalog.Repos[0]
	if !reflect.DeepEqual(repo.Tags, []string{"backend", "critical"}) {
		t.Fatalf("catalog tags = %v, want %v", repo.Tags, []string{"backend", "critical"})
	}
	if repo.RemoteURL != "https://github.com/acme/new-api" {
		t.Fatalf("catalog remote_url = %q, want %q", repo.RemoteURL, "https://github.com/acme/new-api")
	}
	if len(repo.Locations) != 2 {
		t.Fatalf("catalog locations count = %d, want 2", len(repo.Locations))
	}
}

func TestLoadCatalog_NonExistentCreatesEmpty(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join(t.TempDir(), "catalog.yaml")
	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	if catalog.Version != CatalogVersionV1 {
		t.Fatalf("catalog version = %q, want %q", catalog.Version, CatalogVersionV1)
	}
	if len(catalog.Repos) != 0 {
		t.Fatalf("catalog repo count = %d, want 0", len(catalog.Repos))
	}
	if len(catalog.Roots) != 0 {
		t.Fatalf("catalog root count = %d, want 0", len(catalog.Roots))
	}
}

func TestLoadSaveCatalog_RoundTrip(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join(t.TempDir(), "catalog.yaml")
	scopeRoot := filepath.Join(t.TempDir(), "scope")
	catalog := &Catalog{
		Version:   CatalogVersionV1,
		ScopeRoot: scopeRoot,
		Roots: []CatalogRoot{
			{
				Path:          filepath.Join(scopeRoot, "repos"),
				LastScannedAt: time.Now().UTC(),
			},
		},
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/worker",
				RemoteURL: "https://github.com/acme/worker",
				Tags:      []string{"ops"},
				Locations: []RepoLocation{
					{Path: filepath.Join(scopeRoot, "repos", "worker"), LastSeenAt: time.Now().UTC()},
				},
			},
		},
	}

	if err := SaveCatalog(catalogPath, catalog); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}

	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(raw), "path: repos/worker") {
		t.Fatalf("saved catalog should contain relative repo path, got:\n%s", string(raw))
	}

	loaded, err := LoadCatalogWithScope(catalogPath, scopeRoot)
	if err != nil {
		t.Fatalf("LoadCatalogWithScope() error = %v", err)
	}

	if loaded.Version != CatalogVersionV1 {
		t.Fatalf("loaded catalog version = %q, want %q", loaded.Version, CatalogVersionV1)
	}
	if len(loaded.Repos) != 1 {
		t.Fatalf("loaded catalog repo count = %d, want 1", len(loaded.Repos))
	}
	if len(loaded.Roots) != 1 {
		t.Fatalf("loaded catalog root count = %d, want 1", len(loaded.Roots))
	}
	if !reflect.DeepEqual(loaded.Repos[0].Tags, []string{"ops"}) {
		t.Fatalf("loaded tags = %v, want %v", loaded.Repos[0].Tags, []string{"ops"})
	}
	if len(loaded.Repos[0].Locations) != 1 {
		t.Fatalf("loaded locations count = %d, want 1", len(loaded.Repos[0].Locations))
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatal("loaded updated_at should not be zero")
	}
	if loaded.Repos[0].Locations[0].Path != filepath.Join(scopeRoot, "repos", "worker") {
		t.Fatalf(
			"loaded location path = %q, want %q",
			loaded.Repos[0].Locations[0].Path,
			filepath.Join(scopeRoot, "repos", "worker"),
		)
	}
}

func TestLoadCatalogWithScope_RewritesRelativePathsAgainstCurrentScopeRoot(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join(t.TempDir(), "catalog.yaml")
	originalScopeRoot := filepath.Join(t.TempDir(), "scope-old")
	relocatedScopeRoot := filepath.Join(t.TempDir(), "scope-new")
	now := time.Now().UTC()

	catalog := &Catalog{
		Version:   CatalogVersionV1,
		ScopeRoot: originalScopeRoot,
		Roots: []CatalogRoot{
			{
				Path:          filepath.Join(originalScopeRoot, "repos"),
				LastScannedAt: now,
			},
		},
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/worker",
				RemoteURL: "https://github.com/acme/worker",
				Locations: []RepoLocation{
					{
						Path:       filepath.Join(originalScopeRoot, "repos", "worker"),
						LastSeenAt: now,
					},
				},
			},
		},
	}
	if err := SaveCatalog(catalogPath, catalog); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}

	loaded, err := LoadCatalogWithScope(catalogPath, relocatedScopeRoot)
	if err != nil {
		t.Fatalf("LoadCatalogWithScope() error = %v", err)
	}

	if loaded.Roots[0].Path != filepath.Join(relocatedScopeRoot, "repos") {
		t.Fatalf("loaded root path = %q, want %q", loaded.Roots[0].Path, filepath.Join(relocatedScopeRoot, "repos"))
	}
	if loaded.Repos[0].Locations[0].Path != filepath.Join(relocatedScopeRoot, "repos", "worker") {
		t.Fatalf(
			"loaded location path = %q, want %q",
			loaded.Repos[0].Locations[0].Path,
			filepath.Join(relocatedScopeRoot, "repos", "worker"),
		)
	}
}

func TestMergeCatalogs_PreservesRepoIndexAcrossOutOfOrderAppends(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	sharedRepoID := "example.com/team/shared"

	base := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID:   sharedRepoID,
				Tags: []string{"local"},
				Locations: []RepoLocation{
					{
						Path:       "/repos/local/shared",
						LastSeenAt: now,
					},
				},
			},
		},
	}

	imported := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID:   "github.com/example/aaa",
				Tags: []string{"external"},
				Locations: []RepoLocation{
					{
						Path:       "/repos/external/aaa",
						LastSeenAt: now,
					},
				},
			},
			{
				ID:   sharedRepoID,
				Tags: []string{"w:zucli"},
				Locations: []RepoLocation{
					{
						Path:       "/repos/external/shared",
						LastSeenAt: now.Add(time.Minute),
					},
				},
			},
		},
	}

	merged := MergeCatalogs(base, imported)

	index, err := ResolveRepoIndex(merged, sharedRepoID)
	if err != nil {
		t.Fatalf("ResolveRepoIndex() error = %v", err)
	}

	repo := merged.Repos[index]
	if !reflect.DeepEqual(repo.Tags, []string{"local", "w:zucli"}) {
		t.Fatalf("merged tags = %v, want %v", repo.Tags, []string{"local", "w:zucli"})
	}
	if len(repo.Locations) != 2 {
		t.Fatalf("merged locations = %d, want 2", len(repo.Locations))
	}

	otherIndex, err := ResolveRepoIndex(merged, "github.com/example/aaa")
	if err != nil {
		t.Fatalf("ResolveRepoIndex(other) error = %v", err)
	}
	if !reflect.DeepEqual(merged.Repos[otherIndex].Tags, []string{"external"}) {
		t.Fatalf("other repo tags = %v, want %v", merged.Repos[otherIndex].Tags, []string{"external"})
	}
}

func TestLoadCatalog_RejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	catalogPath := filepath.Join(t.TempDir(), "catalog.yaml")
	invalid := `version: "2"
updated_at: 2026-03-06T10:00:00Z
roots: []
repos: []
`
	if err := os.WriteFile(catalogPath, []byte(invalid), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadCatalog(catalogPath)
	if err == nil {
		t.Fatal("LoadCatalog() expected error")
	}
	if !strings.Contains(err.Error(), `unsupported catalog version "2"`) {
		t.Fatalf("LoadCatalog() error = %q, want unsupported version message", err.Error())
	}
}
