package fsfind

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// GitDirectoriesStrict finds repository roots and returns any discovery error.
// Unlike GitDirectoriesTree, it is intended for operations that must fail closed.
func GitDirectoriesStrict(paths ...string) ([]string, error) {
	return GitDirectoriesStrictContext(context.Background(), paths...)
}

// GitDirectoriesStrictContext finds repository roots, honoring cancellation and
// failing if a root or any traversed path cannot be inspected.
func GitDirectoriesStrictContext(ctx context.Context, paths ...string) ([]string, error) {
	return gitDirectoriesStrictWithWalk(ctx, filepath.WalkDir, paths...)
}

type strictWalkDirFunc func(root string, fn fs.WalkDirFunc) error

func gitDirectoriesStrictWithWalk(ctx context.Context, walkDir strictWalkDirFunc, roots ...string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	repoSet := make(map[string]struct{}, len(roots))
	repoPaths := make([]string, 0, len(roots))
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resolvedRoot, err := resolveStrictRoot(root)
		if err != nil {
			return nil, err
		}

		err = walkDir(resolvedRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry == nil || !entry.IsDir() {
				return nil
			}

			isRepoRoot, err := pathContainsGitRepoMarker(path)
			if err != nil {
				return err
			}
			if !isRepoRoot {
				return nil
			}

			cleanPath := filepath.Clean(path)
			if _, ok := repoSet[cleanPath]; !ok {
				repoSet[cleanPath] = struct{}{}
				repoPaths = append(repoPaths, cleanPath)
			}
			return filepath.SkipDir
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(repoPaths)
	return repoPaths, nil
}

func resolveStrictRoot(root string) (string, error) {
	cleanRoot := filepath.Clean(root)
	absoluteRoot, err := filepath.Abs(cleanRoot)
	if err != nil {
		return "", err
	}

	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository search root %q is not a directory", root)
	}

	return filepath.Clean(resolvedRoot), nil
}
