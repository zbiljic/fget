package fconfig

import (
	"path/filepath"
	"strings"
	"time"
)

type SyncOptions struct {
	Roots []string
	Prune bool
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

	paths, err := find(opts.Roots...)
	if err != nil {
		return err
	}

	seen := make(map[string]map[string]struct{}, len(paths))
	for _, discoveredPath := range paths {
		repo, err := inspect(discoveredPath)
		if err != nil || repo.ID == "" {
			continue
		}

		repoPath := filepath.Clean(repo.Path)
		if repoPath == "." || repoPath == "" {
			repoPath = filepath.Clean(discoveredPath)
		}

		catalog.Upsert(RepoEntry{
			ID:        repo.ID,
			RemoteURL: repo.RemoteURL,
			Locations: []RepoLocation{
				{
					Path:       repoPath,
					LastSeenAt: now,
				},
			},
		})

		repoSeen := seen[repo.ID]
		if repoSeen == nil {
			repoSeen = make(map[string]struct{})
			seen[repo.ID] = repoSeen
		}
		repoSeen[repoPath] = struct{}{}
	}

	if opts.Prune {
		catalog.PruneLocationsUnderRoots(scannedRoots, seen)
	}

	return nil
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
