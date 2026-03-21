package fconfig

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SyncOptions struct {
	Roots    []string
	Prune    bool
	Workers  int
	Progress func(processed, total int)
}

type RepoMetadata struct {
	ID        string
	Path      string
	RemoteURL string
}

type (
	Finder    func(roots ...string) ([]string, error)
	Inspector func(path string) (RepoMetadata, error)
)

func SyncCatalog(
	catalog *Catalog,
	opts SyncOptions,
	find Finder,
	inspect Inspector,
	now time.Time,
) error {
	scannedRoots := normalizePaths(opts.Roots)
	for _, root := range scannedRoots {
		catalog.UpsertRoot(root, now)
	}

	paths, err := find(scannedRoots...)
	if err != nil {
		return err
	}
	progress := synchronizedProgressReporter(opts.Progress)
	reportSyncProgress(progress, 0, len(paths))

	repoIndex := make(map[string]int, len(catalog.Repos))
	for i := range catalog.Repos {
		repoIndex[catalog.Repos[i].ID] = i
	}

	seen := make(map[string]map[string]struct{}, len(paths))
	for _, repo := range inspectRepos(paths, inspect, opts.Workers, progress) {
		if !repo.OK || repo.Metadata.ID == "" {
			continue
		}

		discoveredPath := repo.Path
		repoMetadata := repo.Metadata
		repoPath := filepath.Clean(repo.Path)
		if repoPath == "." || repoPath == "" {
			repoPath = filepath.Clean(discoveredPath)
		}

		upsertRepoEntry(catalog, repoIndex, RepoEntry{
			ID:        repoMetadata.ID,
			RemoteURL: repoMetadata.RemoteURL,
			Locations: []RepoLocation{
				{
					Path:       repoPath,
					LastSeenAt: now,
				},
			},
		})

		repoSeen := seen[repoMetadata.ID]
		if repoSeen == nil {
			repoSeen = make(map[string]struct{})
			seen[repoMetadata.ID] = repoSeen
		}
		repoSeen[repoPath] = struct{}{}
	}

	if opts.Prune {
		catalog.PruneLocationsUnderRoots(scannedRoots, seen)
	}
	sort.Slice(catalog.Repos, func(i, j int) bool {
		return catalog.Repos[i].ID < catalog.Repos[j].ID
	})

	return nil
}

type inspectedRepo struct {
	Path     string
	Metadata RepoMetadata
	OK       bool
}

func inspectRepos(
	paths []string,
	inspect Inspector,
	workers int,
	progress func(processed, total int),
) []inspectedRepo {
	results := make([]inspectedRepo, len(paths))
	if len(paths) == 0 {
		return results
	}

	workerCount := normalizedSyncWorkerCount(workers, len(paths))
	if workerCount == 1 {
		for i, path := range paths {
			results[i] = inspectRepoPath(path, inspect)

			reportSyncProgress(progress, i+1, len(paths))
		}

		return results
	}

	var processed atomic.Int64
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()

			for i := range jobs {
				path := paths[i]
				results[i] = inspectRepoPath(path, inspect)

				reportSyncProgress(progress, int(processed.Add(1)), len(paths))
			}
		}()
	}

	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results
}

func inspectRepoPath(path string, inspect Inspector) inspectedRepo {
	repo, err := inspect(path)
	if err != nil || repo.ID == "" {
		return inspectedRepo{}
	}

	return inspectedRepo{
		Path:     path,
		Metadata: repo,
		OK:       true,
	}
}

func normalizedSyncWorkerCount(workers, total int) int {
	if total <= 1 {
		return total
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > total {
		return total
	}

	return workers
}

func reportSyncProgress(progress func(processed, total int), processed, total int) {
	if progress == nil {
		return
	}

	progress(processed, total)
}

func synchronizedProgressReporter(progress func(processed, total int)) func(processed, total int) {
	if progress == nil {
		return nil
	}

	var mu sync.Mutex
	return func(processed, total int) {
		mu.Lock()
		defer mu.Unlock()
		progress(processed, total)
	}
}

func upsertRepoEntry(catalog *Catalog, repoIndex map[string]int, entry RepoEntry) {
	entry = normalizeRepoEntry(entry)

	if i, ok := repoIndex[entry.ID]; ok {
		updated := catalog.Repos[i]
		if entry.RemoteURL != "" {
			updated.RemoteURL = entry.RemoteURL
		}
		if len(entry.Tags) > 0 {
			updated.Tags = append([]string{}, entry.Tags...)
		}
		updated.Locations = mergeLocations(updated.Locations, entry.Locations)
		catalog.Repos[i] = normalizeRepoEntry(updated)
		return
	}

	if entry.Tags == nil {
		entry.Tags = []string{}
	}

	catalog.Repos = append(catalog.Repos, normalizeRepoEntry(entry))
	repoIndex[entry.ID] = len(catalog.Repos) - 1
}

func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))

	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}

	return out
}

func isPathUnderAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if isPathUnderRoot(path, root) {
			return true
		}
	}
	return false
}

func isPathUnderRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)

	if path == root {
		return true
	}

	if root == string(filepath.Separator) {
		return true
	}

	sepRoot := root + string(filepath.Separator)
	return strings.HasPrefix(path, sepRoot)
}
