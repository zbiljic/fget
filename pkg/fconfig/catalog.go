package fconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/zbiljic/fget/pkg/vconfig"
)

const (
	CatalogVersionV1 = "1"
)

type Catalog struct {
	Version   string        `yaml:"version" json:"version"`
	UpdatedAt time.Time     `yaml:"updated_at" json:"updated_at"`
	Roots     []CatalogRoot `yaml:"roots" json:"roots"`
	Repos     []RepoEntry   `yaml:"repos" json:"repos"`
	ScopeRoot string        `yaml:"-" json:"-"`
}

type CatalogRoot struct {
	Path          string    `yaml:"path" json:"path"`
	LastScannedAt time.Time `yaml:"last_scanned_at" json:"last_scanned_at"`
}

type RepoEntry struct {
	ID        string         `yaml:"id" json:"id"`
	RemoteURL string         `yaml:"remote_url" json:"remote_url"`
	Tags      []string       `yaml:"tags" json:"tags"`
	Locations []RepoLocation `yaml:"locations" json:"locations"`
}

type RepoLocation struct {
	Path       string    `yaml:"path" json:"path"`
	LastSeenAt time.Time `yaml:"last_seen_at" json:"last_seen_at"`
}

func newCatalog() *Catalog {
	return &Catalog{
		Version: CatalogVersionV1,
		Roots:   []CatalogRoot{},
		Repos:   []RepoEntry{},
	}
}

func (c *Catalog) Upsert(entry RepoEntry) {
	entry = normalizeRepoEntry(entry)

	for i := range c.Repos {
		if c.Repos[i].ID != entry.ID {
			continue
		}

		updated := c.Repos[i]
		if entry.RemoteURL != "" {
			updated.RemoteURL = entry.RemoteURL
		}
		if len(entry.Tags) > 0 {
			updated.Tags = append([]string{}, entry.Tags...)
		}
		updated.Locations = mergeLocations(updated.Locations, entry.Locations)
		c.Repos[i] = normalizeRepoEntry(updated)
		return
	}

	if entry.Tags == nil {
		entry.Tags = []string{}
	}
	c.Repos = append(c.Repos, normalizeRepoEntry(entry))
	sort.Slice(c.Repos, func(i, j int) bool {
		return c.Repos[i].ID < c.Repos[j].ID
	})
}

func (c *Catalog) UpsertRoot(path string, scannedAt time.Time) {
	path = filepath.Clean(path)

	for i := range c.Roots {
		if c.Roots[i].Path == path {
			c.Roots[i].LastScannedAt = scannedAt
			return
		}
	}

	c.Roots = append(c.Roots, CatalogRoot{
		Path:          path,
		LastScannedAt: scannedAt,
	})
	sort.Slice(c.Roots, func(i, j int) bool {
		return c.Roots[i].Path < c.Roots[j].Path
	})
}

func (c *Catalog) PruneLocationsUnderRoots(scannedRoots []string, seen map[string]map[string]struct{}) {
	filteredRepos := make([]RepoEntry, 0, len(c.Repos))

	for _, repo := range c.Repos {
		filteredLocations := make([]RepoLocation, 0, len(repo.Locations))
		repoSeen := seen[repo.ID]

		for _, loc := range repo.Locations {
			loc.Path = filepath.Clean(loc.Path)

			if _, ok := repoSeen[loc.Path]; ok {
				filteredLocations = append(filteredLocations, loc)
				continue
			}

			if !isPathUnderAnyRoot(loc.Path, scannedRoots) {
				if !pathExists(loc.Path) {
					continue
				}
				filteredLocations = append(filteredLocations, loc)
				continue
			}
		}

		if len(filteredLocations) == 0 {
			continue
		}

		repo.Locations = filteredLocations
		filteredRepos = append(filteredRepos, normalizeRepoEntry(repo))
	}

	c.Repos = filteredRepos
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func LoadCatalog(path string) (*Catalog, error) {
	return LoadCatalogWithScope(path, "")
}

func LoadCatalogWithScope(path, scopeRoot string) (*Catalog, error) {
	if !fileExists(path) {
		catalog := newCatalog()
		catalog.ScopeRoot = cleanScopeRoot(scopeRoot)
		return catalog, nil
	}

	version, err := vconfig.GetVersion(path)
	if err != nil {
		return nil, err
	}

	switch version {
	case "", CatalogVersionV1:
		catalog, err := vconfig.LoadConfig[Catalog](path)
		if err != nil {
			return nil, err
		}
		catalog.ScopeRoot = cleanScopeRoot(scopeRoot)
		normalizeLoadedCatalog(catalog)
		return catalog, nil
	default:
		return nil, fmt.Errorf("unsupported catalog version %q", version)
	}
}

func SaveCatalog(path string, catalog *Catalog) error {
	if catalog == nil {
		return errors.New("nil catalog")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	catalog.ScopeRoot = cleanScopeRoot(catalog.ScopeRoot)
	catalog.Version = CatalogVersionV1
	catalog.UpdatedAt = time.Now().UTC()
	normalizeLoadedCatalog(catalog)

	return vconfig.SaveConfig(snapshotCatalogForSave(catalog), path)
}

func MergeCatalogs(catalogs ...*Catalog) *Catalog {
	merged := newCatalog()
	repoIndex := make(map[string]int)

	for _, catalog := range catalogs {
		if catalog == nil {
			continue
		}

		normalizeLoadedCatalog(catalog)
		for _, root := range catalog.Roots {
			merged.UpsertRoot(root.Path, root.LastScannedAt)
		}

		for _, repo := range catalog.Repos {
			mergeCatalogRepo(merged, repoIndex, repo)
		}
	}

	return merged
}

func normalizeLoadedCatalog(c *Catalog) {
	if c.Version == "" {
		c.Version = CatalogVersionV1
	}
	if c.Roots == nil {
		c.Roots = []CatalogRoot{}
	}
	if c.Repos == nil {
		c.Repos = []RepoEntry{}
	}

	rootMap := make(map[string]CatalogRoot, len(c.Roots))
	for _, root := range c.Roots {
		root = normalizeLoadedCatalogRoot(root, c.ScopeRoot)
		existing, ok := rootMap[root.Path]
		if !ok || root.LastScannedAt.After(existing.LastScannedAt) {
			rootMap[root.Path] = root
		}
	}
	c.Roots = c.Roots[:0]
	for _, root := range rootMap {
		c.Roots = append(c.Roots, root)
	}
	sort.Slice(c.Roots, func(i, j int) bool {
		return c.Roots[i].Path < c.Roots[j].Path
	})

	for i := range c.Repos {
		c.Repos[i] = normalizeLoadedRepoEntry(c.ScopeRoot, c.Repos[i])
	}
	sort.Slice(c.Repos, func(i, j int) bool {
		return c.Repos[i].ID < c.Repos[j].ID
	})
}

func normalizeRepoEntry(repo RepoEntry) RepoEntry {
	if repo.Tags == nil {
		repo.Tags = []string{}
	}
	repo.Locations = mergeLocations(nil, repo.Locations)
	return repo
}

func normalizeLoadedRepoEntry(scopeRoot string, repo RepoEntry) RepoEntry {
	if repo.Tags == nil {
		repo.Tags = []string{}
	}
	repo.Locations = mergeLoadedLocations(scopeRoot, nil, repo.Locations)
	return repo
}

func mergeLocations(existing, incoming []RepoLocation) []RepoLocation {
	locMap := make(map[string]RepoLocation, len(existing)+len(incoming))

	for _, loc := range existing {
		loc.Path = filepath.Clean(loc.Path)
		locMap[loc.Path] = loc
	}
	for _, loc := range incoming {
		loc.Path = filepath.Clean(loc.Path)
		prev, ok := locMap[loc.Path]
		if !ok || loc.LastSeenAt.After(prev.LastSeenAt) {
			locMap[loc.Path] = loc
		}
	}

	merged := make([]RepoLocation, 0, len(locMap))
	for _, loc := range locMap {
		merged = append(merged, loc)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Path < merged[j].Path
	})

	return merged
}

func mergeLoadedLocations(scopeRoot string, existing, incoming []RepoLocation) []RepoLocation {
	locMap := make(map[string]RepoLocation, len(existing)+len(incoming))

	for _, loc := range existing {
		loc = normalizeLoadedRepoLocation(loc, scopeRoot)
		locMap[loc.Path] = loc
	}
	for _, loc := range incoming {
		loc = normalizeLoadedRepoLocation(loc, scopeRoot)
		prev, ok := locMap[loc.Path]
		if !ok || loc.LastSeenAt.After(prev.LastSeenAt) {
			locMap[loc.Path] = loc
		}
	}

	merged := make([]RepoLocation, 0, len(locMap))
	for _, loc := range locMap {
		merged = append(merged, loc)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Path < merged[j].Path
	})

	return merged
}

func normalizeLoadedCatalogRoot(root CatalogRoot, scopeRoot string) CatalogRoot {
	root.Path = resolveStoredCatalogPath(root.Path, scopeRoot)
	return root
}

func normalizeLoadedRepoLocation(location RepoLocation, scopeRoot string) RepoLocation {
	location.Path = resolveStoredCatalogPath(location.Path, scopeRoot)
	return location
}

func resolveStoredCatalogPath(path, scopeRoot string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) || scopeRoot == "" {
		return filepath.Clean(path)
	}

	return filepath.Clean(filepath.Join(scopeRoot, path))
}

func snapshotCatalogForSave(catalog *Catalog) *Catalog {
	snapshot := &Catalog{
		Version:   CatalogVersionV1,
		UpdatedAt: catalog.UpdatedAt,
		Roots:     make([]CatalogRoot, 0, len(catalog.Roots)),
		Repos:     make([]RepoEntry, 0, len(catalog.Repos)),
	}

	for _, root := range catalog.Roots {
		snapshot.Roots = append(snapshot.Roots, CatalogRoot{
			Path:          serializeCatalogPath(root.Path, catalog.ScopeRoot),
			LastScannedAt: root.LastScannedAt,
		})
	}

	for _, repo := range catalog.Repos {
		serialized := RepoEntry{
			ID:        repo.ID,
			RemoteURL: repo.RemoteURL,
			Tags:      append([]string{}, repo.Tags...),
			Locations: make([]RepoLocation, 0, len(repo.Locations)),
		}
		for _, location := range repo.Locations {
			serialized.Locations = append(serialized.Locations, RepoLocation{
				Path:       serializeCatalogPath(location.Path, catalog.ScopeRoot),
				LastSeenAt: location.LastSeenAt,
			})
		}
		snapshot.Repos = append(snapshot.Repos, normalizeRepoEntry(serialized))
	}

	normalizeStoredCatalog(snapshot)
	return snapshot
}

func serializeCatalogPath(path, scopeRoot string) string {
	path = filepath.Clean(path)
	if rel, ok := relativeToScope(path, scopeRoot); ok {
		return rel
	}

	return path
}

func normalizeStoredCatalog(c *Catalog) {
	if c.Version == "" {
		c.Version = CatalogVersionV1
	}
	if c.Roots == nil {
		c.Roots = []CatalogRoot{}
	}
	if c.Repos == nil {
		c.Repos = []RepoEntry{}
	}

	rootMap := make(map[string]CatalogRoot, len(c.Roots))
	for _, root := range c.Roots {
		root.Path = filepath.Clean(root.Path)
		existing, ok := rootMap[root.Path]
		if !ok || root.LastScannedAt.After(existing.LastScannedAt) {
			rootMap[root.Path] = root
		}
	}
	c.Roots = c.Roots[:0]
	for _, root := range rootMap {
		c.Roots = append(c.Roots, root)
	}
	sort.Slice(c.Roots, func(i, j int) bool {
		return c.Roots[i].Path < c.Roots[j].Path
	})

	for i := range c.Repos {
		c.Repos[i] = normalizeRepoEntry(c.Repos[i])
	}
	sort.Slice(c.Repos, func(i, j int) bool {
		return c.Repos[i].ID < c.Repos[j].ID
	})
}

func relativeToScope(path, scopeRoot string) (string, bool) {
	if path == "" || scopeRoot == "" {
		return "", false
	}

	path = filepath.Clean(path)
	scopeRoot = filepath.Clean(scopeRoot)
	if !isPathUnderRoot(path, scopeRoot) {
		return "", false
	}

	rel, err := filepath.Rel(scopeRoot, path)
	if err != nil {
		return "", false
	}

	return rel, true
}

func cleanScopeRoot(scopeRoot string) string {
	if scopeRoot == "" {
		return ""
	}

	return filepath.Clean(scopeRoot)
}

func mergeCatalogRepo(catalog *Catalog, repoIndex map[string]int, repo RepoEntry) {
	repo = normalizeRepoEntry(repo)

	if i, ok := repoIndex[repo.ID]; ok {
		updated := catalog.Repos[i]
		if repo.RemoteURL != "" {
			updated.RemoteURL = repo.RemoteURL
		}
		updated.Tags = normalizeTags(append(updated.Tags, repo.Tags...))
		updated.Locations = mergeLocations(updated.Locations, repo.Locations)
		catalog.Repos[i] = normalizeRepoEntry(updated)
		return
	}

	catalog.Repos = append(catalog.Repos, normalizeRepoEntry(repo))
	repoIndex[repo.ID] = len(catalog.Repos) - 1
	sort.Slice(catalog.Repos, func(i, j int) bool {
		return catalog.Repos[i].ID < catalog.Repos[j].ID
	})
}
