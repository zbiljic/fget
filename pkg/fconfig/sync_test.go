package fconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInspectRepoPath(t *testing.T) {
	t.Parallel()

	inspectErr := errors.New("cannot read repository metadata")
	tests := []struct {
		name    string
		path    string
		inspect Inspector
		wantErr error
	}{
		{
			name: "inspector error",
			path: "/repos/src/unreadable",
			inspect: func(string) (RepoMetadata, error) {
				return RepoMetadata{}, inspectErr
			},
			wantErr: inspectErr,
		},
		{
			name: "empty repository ID",
			path: "/repos/src/malformed",
			inspect: func(path string) (RepoMetadata, error) {
				return RepoMetadata{Path: path}, nil
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := inspectRepoPath(tt.path, tt.inspect)
			if got.Path != tt.path {
				t.Fatalf("inspectRepoPath() path = %q, want %q", got.Path, tt.path)
			}
			if got.Err == nil {
				t.Fatal("inspectRepoPath() error = nil, want error")
			}
			if !strings.Contains(got.Err.Error(), tt.path) {
				t.Fatalf("inspectRepoPath() error = %q, want path %q", got.Err, tt.path)
			}
			if tt.wantErr != nil && !errors.Is(got.Err, tt.wantErr) {
				t.Fatalf("inspectRepoPath() error = %v, want wrapped %v", got.Err, tt.wantErr)
			}
			if got.OK {
				t.Fatal("inspectRepoPath() OK = true, want false")
			}
		})
	}
}

func TestSyncCatalog_InspectionFailuresLeaveCatalogUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paths     []string
		failed    map[string]error
		workers   int
		prune     bool
		wantOrder []string
	}{
		{
			name: "single failure without prune",
			paths: []string{
				"/repos/src/unreadable",
			},
			failed: map[string]error{
				"/repos/src/unreadable": errors.New("permission denied"),
			},
			workers:   1,
			wantOrder: []string{"/repos/src/unreadable"},
		},
		{
			name: "multiple failures with prune and workers",
			paths: []string{
				"/repos/src/zeta",
				"/repos/src/healthy",
				"/repos/src/alpha",
			},
			failed: map[string]error{
				"/repos/src/zeta":  errors.New("bad config"),
				"/repos/src/alpha": errors.New("unreadable config"),
			},
			workers:   3,
			prune:     true,
			wantOrder: []string{"/repos/src/alpha", "/repos/src/zeta"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldRootScan := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
			oldRepoSeen := oldRootScan.Add(time.Hour)
			catalog := &Catalog{
				Version:   CatalogVersionV1,
				UpdatedAt: oldRootScan.Add(-time.Hour),
				ScopeRoot: "/repos",
				Roots: []CatalogRoot{
					{Path: "/repos/src", LastScannedAt: oldRootScan},
				},
				Repos: []RepoEntry{
					{
						ID:        "github.com/acme/existing",
						RemoteURL: "https://example.com/acme/existing",
						Tags:      []string{"keep"},
						Locations: []RepoLocation{
							{Path: "/repos/src/existing", LastSeenAt: oldRepoSeen},
						},
					},
				},
			}
			before := cloneCatalogForSyncTest(catalog)

			var progressEvents []struct {
				processed int
				total     int
			}
			err := SyncCatalog(context.Background(), catalog, SyncOptions{
				Roots:   []string{"/repos/src"},
				Workers: tt.workers,
				Prune:   tt.prune,
				Progress: func(processed, total int) {
					progressEvents = append(progressEvents, struct {
						processed int
						total     int
					}{processed: processed, total: total})
				},
			}, func(roots ...string) ([]string, error) {
				return append([]string{}, tt.paths...), nil
			}, func(path string) (RepoMetadata, error) {
				if err := tt.failed[path]; err != nil {
					return RepoMetadata{}, err
				}
				return RepoMetadata{
					ID:        "github.com/acme/healthy",
					Path:      path,
					RemoteURL: "https://example.com/acme/healthy",
				}, nil
			}, oldRootScan.Add(24*time.Hour))
			if err == nil {
				t.Fatal("SyncCatalog() error = nil, want inspection error")
			}

			lastIndex := -1
			for _, path := range tt.wantOrder {
				index := strings.Index(err.Error(), path)
				if index == -1 {
					t.Fatalf("SyncCatalog() error = %q, want path %q", err, path)
				}
				if index <= lastIndex {
					t.Fatalf("SyncCatalog() error = %q, want paths in order %v", err, tt.wantOrder)
				}
				lastIndex = index
			}
			if !reflect.DeepEqual(catalog, before) {
				t.Fatalf("catalog changed after inspection failure:\n got: %#v\nwant: %#v", catalog, before)
			}

			maxProcessed := 0
			for _, event := range progressEvents {
				if event.total != len(tt.paths) {
					t.Fatalf("progress total = %d, want %d", event.total, len(tt.paths))
				}
				if event.processed > maxProcessed {
					maxProcessed = event.processed
				}
			}
			if maxProcessed != len(tt.paths) {
				t.Fatalf("progress reached %d, want %d; events = %#v", maxProcessed, len(tt.paths), progressEvents)
			}
		})
	}
}

func cloneCatalogForSyncTest(catalog *Catalog) *Catalog {
	clone := *catalog
	clone.Roots = append([]CatalogRoot{}, catalog.Roots...)
	clone.Repos = make([]RepoEntry, len(catalog.Repos))
	for i, repo := range catalog.Repos {
		clone.Repos[i] = repo
		clone.Repos[i].Tags = append([]string{}, repo.Tags...)
		clone.Repos[i].Locations = append([]RepoLocation{}, repo.Locations...)
	}

	return &clone
}

func TestSyncCatalog_ReportsProgress(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{Version: CatalogVersionV1}

	var progressEvents []struct {
		Processed int
		Total     int
	}

	now := time.Now().UTC()
	find := func(roots ...string) ([]string, error) {
		return []string{"/repos/src/one", "/repos/src/two"}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		return RepoMetadata{
			ID:        "github.com/acme/" + path[len(path)-3:],
			Path:      path,
			RemoteURL: "https://example.com/" + path[len(path)-3:],
		}, nil
	}

	err := SyncCatalog(context.Background(), catalog, SyncOptions{
		Roots: []string{"/repos/src"},
		Progress: func(processed, total int) {
			progressEvents = append(progressEvents, struct {
				Processed int
				Total     int
			}{Processed: processed, Total: total})
		},
	}, find, inspect, now)
	if err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	wantEvents := []struct {
		Processed int
		Total     int
	}{
		{Processed: 0, Total: 2},
		{Processed: 1, Total: 2},
		{Processed: 2, Total: 2},
	}
	if !reflect.DeepEqual(progressEvents, wantEvents) {
		t.Fatalf("progress events = %v, want %v", progressEvents, wantEvents)
	}
}

func TestSyncCatalog_InspectsRepositoriesConcurrently(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{Version: CatalogVersionV1}
	inspectStarted := make(chan struct{}, 2)
	releaseInspect := make(chan struct{})
	done := make(chan error, 1)

	find := func(roots ...string) ([]string, error) {
		return []string{"/repos/src/one", "/repos/src/two"}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		inspectStarted <- struct{}{}
		<-releaseInspect

		return RepoMetadata{
			ID:   path,
			Path: path,
		}, nil
	}

	go func() {
		done <- SyncCatalog(context.Background(), catalog, SyncOptions{
			Roots:   []string{"/repos/src"},
			Workers: 2,
		}, find, inspect, time.Now().UTC())
	}()

	select {
	case <-inspectStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first inspect call did not start")
	}

	select {
	case <-inspectStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("second inspect call did not start before the first completed")
	}

	close(releaseInspect)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SyncCatalog() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SyncCatalog() did not return")
	}
}

func TestSyncCatalog_CancelStopsFurtherInspection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	catalog := &Catalog{Version: CatalogVersionV1}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	var inspectCalls atomic.Int64

	find := func(roots ...string) ([]string, error) {
		return []string{"/repos/src/one", "/repos/src/two", "/repos/src/three"}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		call := inspectCalls.Add(1)
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		if call == 2 {
			secondStarted <- struct{}{}
		}

		return RepoMetadata{
			ID:   path,
			Path: path,
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- SyncCatalog(ctx, catalog, SyncOptions{
			Roots:   []string{"/repos/src"},
			Workers: 1,
		}, find, inspect, time.Now().UTC())
	}()

	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first inspect call did not start")
	}

	cancel()
	close(releaseFirst)

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("SyncCatalog() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SyncCatalog() did not return after cancellation")
	}

	select {
	case <-secondStarted:
		t.Fatal("second inspect call started after cancellation")
	default:
	}
}

func TestSyncCatalog_FinderReceivesNormalizedRoots(t *testing.T) {
	t.Parallel()

	catalog := &Catalog{Version: CatalogVersionV1}

	var gotRoots []string
	find := func(roots ...string) ([]string, error) {
		gotRoots = append([]string{}, roots...)
		return []string{}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		t.Fatalf("inspect should not be called")
		return RepoMetadata{}, nil
	}

	err := SyncCatalog(context.Background(), catalog, SyncOptions{
		Roots: []string{"/repos/src", "/repos/src", ".", "/repos/other/../other"},
	}, find, inspect, time.Now().UTC())
	if err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	wantRoots := []string{"/repos/src", "/repos/other"}
	if !reflect.DeepEqual(gotRoots, wantRoots) {
		t.Fatalf("find roots = %v, want %v", gotRoots, wantRoots)
	}
}

func TestSyncCatalog_PruneOnlyTouchesScannedRoots(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	scannedRootPath := filepath.Join(baseDir, "src")
	scannedRepoPath := filepath.Join(scannedRootPath, "kept")
	unscannedRootPath := filepath.Join(baseDir, "wtopics")
	unscannedRepoPath := filepath.Join(unscannedRootPath, "kept")
	removedRepoPath := filepath.Join(scannedRootPath, "remove-me")

	for _, path := range []string{scannedRepoPath, unscannedRepoPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
	}

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Roots: []CatalogRoot{
			{Path: scannedRootPath, LastScannedAt: time.Now().UTC().Add(-2 * time.Hour)},
			{Path: unscannedRootPath, LastScannedAt: time.Now().UTC().Add(-2 * time.Hour)},
		},
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/kept",
				RemoteURL: "https://github.com/acme/api",
				Tags:      []string{"service"},
				Locations: []RepoLocation{
					{Path: scannedRepoPath, LastSeenAt: time.Now().UTC().Add(-time.Hour)},
					{Path: unscannedRepoPath, LastSeenAt: time.Now().UTC().Add(-time.Hour)},
				},
			},
			{
				ID:        "github.com/acme/remove-me",
				RemoteURL: "https://github.com/acme/stale",
				Tags:      []string{"legacy"},
				Locations: []RepoLocation{
					{Path: removedRepoPath, LastSeenAt: time.Now().UTC().Add(-time.Hour)},
				},
			},
		},
	}

	now := time.Now().UTC()
	find := func(roots ...string) ([]string, error) {
		return []string{scannedRepoPath}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		return RepoMetadata{
			ID:        "github.com/acme/kept",
			Path:      path,
			RemoteURL: "https://github.com/acme/kept",
		}, nil
	}

	err := SyncCatalog(context.Background(), catalog, SyncOptions{
		Roots: []string{scannedRootPath},
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
		if root.Path == scannedRootPath {
			scannedRoot = root
			break
		}
	}
	if scannedRoot.Path == "" {
		t.Fatalf("expected %s root to exist", scannedRootPath)
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
	if repo.Locations[0].Path != scannedRepoPath || repo.Locations[1].Path != unscannedRepoPath {
		t.Fatalf("repo locations = %#v, want paths %q and %q", repo.Locations, scannedRepoPath, unscannedRepoPath)
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

	err := SyncCatalog(context.Background(), catalog, SyncOptions{Prune: false}, find, inspect, now)
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

func TestSyncCatalog_PruneRemovesMissingLocationsOutsideScannedRoots(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	scannedRoot := filepath.Join(baseDir, "scanned")
	scannedPath := filepath.Join(scannedRoot, "repo")
	existingUnscannedPath := filepath.Join(baseDir, "unscanned", "repo")
	missingUnscannedPath := filepath.Join(baseDir, "stale", "repo")

	for _, path := range []string{scannedPath, existingUnscannedPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
	}

	catalog := &Catalog{
		Version: CatalogVersionV1,
		Repos: []RepoEntry{
			{
				ID:        "github.com/acme/shared-repo",
				RemoteURL: "https://github.com/acme/shared-repo",
				Locations: []RepoLocation{
					{Path: existingUnscannedPath, LastSeenAt: time.Now().UTC().Add(-2 * time.Hour)},
					{Path: missingUnscannedPath, LastSeenAt: time.Now().UTC().Add(-2 * time.Hour)},
				},
			},
		},
	}

	find := func(roots ...string) ([]string, error) {
		return []string{scannedPath}, nil
	}
	inspect := func(path string) (RepoMetadata, error) {
		return RepoMetadata{
			ID:        "github.com/acme/shared-repo",
			Path:      path,
			RemoteURL: "https://github.com/acme/shared-repo",
		}, nil
	}

	err := SyncCatalog(context.Background(), catalog, SyncOptions{
		Roots: []string{scannedRoot},
		Prune: true,
	}, find, inspect, time.Now().UTC())
	if err != nil {
		t.Fatalf("SyncCatalog() error = %v", err)
	}

	got := catalog.Repos[0].Locations
	want := []RepoLocation{
		{Path: scannedPath, LastSeenAt: got[0].LastSeenAt},
		{Path: existingUnscannedPath, LastSeenAt: got[1].LastSeenAt},
	}
	if len(got) != len(want) {
		t.Fatalf("location count = %d, want %d; locations = %#v", len(got), len(want), got)
	}
	if got[0].Path != want[0].Path || got[1].Path != want[1].Path {
		t.Fatalf("locations = %#v, want paths %q and %q", got, want[0].Path, want[1].Path)
	}
}
