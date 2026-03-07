package fconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	LinkMatchAny     = "any"
	LinkMatchAll     = "all"
	LinkLayoutRepoID = "repo-id"
)

var (
	errEmptyLinkTags     = errors.New("link.tags must contain at least one tag")
	errInvalidLinkMatch  = errors.New("invalid link match mode")
	errInvalidLinkLayout = errors.New("invalid link layout")
	errNilCatalog        = errors.New("nil catalog")
	errLinkTargetOutside = errors.New("link target path must stay under managed root")
)

type LinkTarget struct {
	RepoID     string
	SourcePath string
	TargetPath string
}

type LinkProblem struct {
	RepoID string
	Err    error
}

type LinkSyncResult struct {
	Created int
	Updated int
	Removed int
	Skipped []LinkProblem
}

func ResolveLinkTargets(catalog *Catalog, spec LinkConfig) ([]LinkTarget, []LinkProblem) {
	if catalog == nil {
		return nil, []LinkProblem{{Err: errNilCatalog}}
	}

	spec, err := normalizeLinkSpec(spec)
	if err != nil {
		return nil, []LinkProblem{{Err: err}}
	}

	targets := make([]LinkTarget, 0, len(catalog.Repos))
	problems := make([]LinkProblem, 0)

	for _, repo := range catalog.Repos {
		if !repoMatchesLinkTags(repo, spec) {
			continue
		}

		sourcePath, err := selectLinkSourcePath(repo, spec.SourceRoot)
		if err != nil {
			problems = append(problems, LinkProblem{RepoID: repo.ID, Err: err})
			continue
		}

		targetPath, err := deriveLinkTargetPath(repo.ID, spec)
		if err != nil {
			problems = append(problems, LinkProblem{RepoID: repo.ID, Err: err})
			continue
		}

		targets = append(targets, LinkTarget{
			RepoID:     repo.ID,
			SourcePath: sourcePath,
			TargetPath: targetPath,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].RepoID < targets[j].RepoID
	})
	sort.Slice(problems, func(i, j int) bool {
		return problems[i].RepoID < problems[j].RepoID
	})

	return targets, problems
}

func normalizeLinkSpec(spec LinkConfig) (LinkConfig, error) {
	spec.Tags = normalizeTags(spec.Tags)
	if len(spec.Tags) == 0 {
		return LinkConfig{}, errEmptyLinkTags
	}

	if spec.Match == "" {
		spec.Match = LinkMatchAny
	}
	switch spec.Match {
	case LinkMatchAny, LinkMatchAll:
	default:
		return LinkConfig{}, fmt.Errorf("%w: %q", errInvalidLinkMatch, spec.Match)
	}

	if spec.Layout == "" {
		spec.Layout = LinkLayoutRepoID
	}
	switch spec.Layout {
	case LinkLayoutRepoID:
	default:
		return LinkConfig{}, fmt.Errorf("%w: %q", errInvalidLinkLayout, spec.Layout)
	}

	if spec.Root == "" {
		spec.Root = "."
	}
	spec.Root = filepath.Clean(spec.Root)
	if spec.SourceRoot != "" {
		spec.SourceRoot = filepath.Clean(spec.SourceRoot)
	}

	return spec, nil
}

func repoMatchesLinkTags(repo RepoEntry, spec LinkConfig) bool {
	repoTags := make(map[string]struct{}, len(repo.Tags))
	for _, tag := range normalizeTags(repo.Tags) {
		repoTags[tag] = struct{}{}
	}

	switch spec.Match {
	case LinkMatchAll:
		for _, tag := range spec.Tags {
			if _, ok := repoTags[tag]; !ok {
				return false
			}
		}
		return true
	default:
		for _, tag := range spec.Tags {
			if _, ok := repoTags[tag]; ok {
				return true
			}
		}
		return false
	}
}

func selectLinkSourcePath(repo RepoEntry, sourceRoot string) (string, error) {
	switch len(repo.Locations) {
	case 0:
		return "", errors.New("repository has no catalog locations")
	case 1:
		return filepath.Clean(repo.Locations[0].Path), nil
	}

	if sourceRoot == "" {
		return "", errors.New("repository has multiple catalog locations; set link.source_root")
	}

	matches := make([]string, 0, len(repo.Locations))
	for _, location := range repo.Locations {
		locationPath := filepath.Clean(location.Path)
		if isPathUnderRoot(locationPath, sourceRoot) {
			matches = append(matches, locationPath)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("repository has no catalog location under source_root %s", sourceRoot)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("repository has multiple locations under source_root %s", sourceRoot)
	}
}

func deriveLinkTargetPath(repoID string, spec LinkConfig) (string, error) {
	switch spec.Layout {
	case LinkLayoutRepoID:
		return filepath.Join(spec.Root, filepath.FromSlash(repoID)), nil
	default:
		return "", fmt.Errorf("%w: %q", errInvalidLinkLayout, spec.Layout)
	}
}

func SyncLinks(root string, targets []LinkTarget) (LinkSyncResult, error) {
	result := LinkSyncResult{}
	root = filepath.Clean(root)

	desiredTargets := make(map[string]LinkTarget, len(targets))
	skipped := make([]LinkProblem, 0)

	for _, target := range targets {
		target.SourcePath = filepath.Clean(target.SourcePath)
		target.TargetPath = filepath.Clean(target.TargetPath)

		if !isPathUnderRoot(target.TargetPath, root) {
			skipped = append(skipped, LinkProblem{
				RepoID: target.RepoID,
				Err:    fmt.Errorf("%w: %s", errLinkTargetOutside, target.TargetPath),
			})
			continue
		}

		desiredTargets[target.TargetPath] = target
	}

	existingSymlinks, err := collectManagedSymlinks(root)
	if err != nil {
		return result, err
	}

	for _, path := range existingSymlinks {
		if _, ok := desiredTargets[path]; ok {
			continue
		}
		if err := os.Remove(path); err != nil {
			return result, err
		}
		result.Removed++
		if err := removeEmptyParents(filepath.Dir(path), root); err != nil {
			return result, err
		}
	}

	desiredPaths := make([]string, 0, len(desiredTargets))
	for path := range desiredTargets {
		desiredPaths = append(desiredPaths, path)
	}
	sort.Strings(desiredPaths)

	for _, targetPath := range desiredPaths {
		target := desiredTargets[targetPath]

		if err := os.MkdirAll(filepath.Dir(target.TargetPath), 0o755); err != nil {
			skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
			continue
		}

		info, err := os.Lstat(target.TargetPath)
		switch {
		case os.IsNotExist(err):
			if err := os.Symlink(target.SourcePath, target.TargetPath); err != nil {
				skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
				continue
			}
			result.Created++
		case err != nil:
			skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
		case info.Mode()&os.ModeSymlink != 0:
			existingTarget, err := os.Readlink(target.TargetPath)
			if err != nil {
				skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
				continue
			}
			if existingTarget == target.SourcePath {
				continue
			}
			if err := os.Remove(target.TargetPath); err != nil {
				skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
				continue
			}
			if err := os.Symlink(target.SourcePath, target.TargetPath); err != nil {
				skipped = append(skipped, LinkProblem{RepoID: target.RepoID, Err: err})
				continue
			}
			result.Updated++
		default:
			skipped = append(skipped, LinkProblem{
				RepoID: target.RepoID,
				Err:    fmt.Errorf("target path %s is occupied by existing non-symlink path", target.TargetPath),
			})
		}
	}

	result.Skipped = skipped
	if len(skipped) > 0 {
		return result, joinLinkProblems(skipped)
	}

	return result, nil
}

func collectManagedSymlinks(root string) ([]string, error) {
	paths := make([]string, 0)

	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return nil, err
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			paths = append(paths, filepath.Clean(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}

func removeEmptyParents(path, root string) error {
	root = filepath.Clean(root)

	for current := filepath.Clean(path); current != root && current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		err := os.Remove(current)
		if err == nil {
			continue
		}
		if os.IsNotExist(err) {
			continue
		}

		entries, readErr := os.ReadDir(current)
		if readErr == nil && len(entries) > 0 {
			return nil
		}
		return err
	}

	return nil
}

func joinLinkProblems(problems []LinkProblem) error {
	errs := make([]error, 0, len(problems))
	for _, problem := range problems {
		if problem.RepoID == "" {
			errs = append(errs, problem.Err)
			continue
		}
		errs = append(errs, fmt.Errorf("%s: %w", problem.RepoID, problem.Err))
	}
	return errors.Join(errs...)
}
