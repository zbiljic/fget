package fconfig

import (
	"reflect"
	"testing"
	"time"
)

func TestSyncCatalog_PruneOnlyTouchesScannedRoots(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Roots: []CatalogRoot{
			{Path: "/repos/src", LastScannedAt: time.Now().UTC().Add(-2 * time.Hour)},
			{Path: "/repos/wtopics", LastScannedAt: time.Now().UTC().Add(-2 * time.Hour)},
		},
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/kept",
				RemoteURL: "https://github.com/acme/api",
				Tags:      []string{"service"},
				Locations: []RepoLocation{
					{Path: "/repos/src/kept", LastSeenAt: time.Now().UTC().Add(-time.Hour)},
					{Path: "/repos/wtopics/kept", LastSeenAt: time.Now().UTC().Add(-time.Hour)},
				},
			},
			{
				ID:        "github.com/acme/remove-me",
				RemoteURL: "https://github.com/acme/stale",
				Tags:      []string{"legacy"},
				Locations: []RepoLocation{
					{Path: "/repos/src/remove-me", LastSeenAt: time.Now().UTC().Add(-time.Hour)},
				},
			},
		},
	}

	now := time.Now().UTC()
	find := func(roots ...string) ([]string, error) {
		return []string{"/repos/src/kept"}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		return RepoMetadata{
			ID:        "github.com/acme/kept",
			Path:      path,
			RemoteURL: "https://github.com/acme/kept",
		}, nil
	}

	err := SyncCatalog(catalog, SyncOptions{
		Roots: []string{"/repos/src"},
		Prune: true,
	}, find, inspect, now)
	if err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	if len(catalog.Repos) != 1 {
		t.Fatalf("catalog repo count = %d, want 1", len(catalog.Repos))
	}
	if len(catalog.Roots) != 2 {
		t.Fatalf("catalog root count = %d, want 2", len(catalog.Roots))
	}
	var scannedRoot CatalogRoot
	for _, root := range catalog.Roots {
		if root.Path == "/repos/src" {
			scannedRoot = root
			break
		}
	}
	if scannedRoot.Path == "" {
		t.Fatal("expected /repos/src root to exist")
	}
	if scannedRoot.LastScannedAt.IsZero() {
		t.Fatal("expected /repos/src last_scanned_at to be set")
	}

	repo := catalog.Repos[0]
	if repo.ID != "github.com/acme/kept" {
		t.Fatalf("repo id = %q, want %q", repo.ID, "github.com/acme/kept")
	}
	if !reflect.DeepEqual(repo.Tags, []string{"service"}) {
		t.Fatalf("repo tags = %v, want %v", repo.Tags, []string{"service"})
	}
	if len(repo.Locations) != 2 {
		t.Fatalf("repo location count = %d, want 2", len(repo.Locations))
	}
	if repo.Locations[0].LastSeenAt.IsZero() && repo.Locations[1].LastSeenAt.IsZero() {
		t.Fatalf("expected at least one updated location timestamp")
	}
}

func TestSyncCatalog_NoPruneKeepsMissingLocations(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/stale",
				RemoteURL: "https://github.com/acme/stale",
				Locations: []RepoLocation{
					{Path: "/repos/src/stale", LastSeenAt: time.Now().UTC().Add(-time.Hour)},
				},
			},
		},
	}

	now := time.Now().UTC()
	find := func(roots ...string) ([]string, error) {
		return []string{}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		t.Fatalf("inspect should not be called")
		return RepoMetadata{}, nil
	}

	err := SyncCatalog(catalog, SyncOptions{Prune: false}, find, inspect, now)
	if err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	if len(catalog.Repos) != 1 {
		t.Fatalf("catalog repo count = %d, want 1", len(catalog.Repos))
	}
	if len(catalog.Repos[0].Locations) != 1 {
		t.Fatalf("location count = %d, want 1", len(catalog.Repos[0].Locations))
	}
}
