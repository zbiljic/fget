package fconfig

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

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

	err := SyncCatalog(context.Background(), catalog, SyncOptions{
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
