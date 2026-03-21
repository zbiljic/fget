package fsfind

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charlievieth/fastwalk"
	art "github.com/plar/go-adaptive-radix-tree/v2"
)

var ErrNotGitRepository = errors.New("not a git repository")

func GitRootPath(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}

		current = parent
	}

	return "", fmt.Errorf("%w: %s", ErrNotGitRepository, path)
}

func GitDirectoriesTree(paths ...string) (art.Tree, error) {
	return GitDirectoriesTreeContext(context.Background(), paths...)
}

func GitDirectoriesTreeContext(ctx context.Context, paths ...string) (art.Tree, error) {
	tree := art.New()
	var mu sync.Mutex

	for _, rootPath := range paths {
		if err := gitDirectoriesUnderRoot(ctx, tree, &mu, rootPath); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func gitDirectoriesUnderRoot(ctx context.Context, tree art.Tree, mu *sync.Mutex, rootPath string) error {
	conf := fastwalk.DefaultConfig.Copy()

	return fastwalk.Walk(conf, rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if walkErr != nil {
			return nil
		}

		if entry == nil || !entry.IsDir() {
			return nil
		}

		isRepoRoot, err := pathContainsGitRepoMarker(path)
		if err != nil {
			return nil
		}
		if !isRepoRoot {
			return nil
		}

		mu.Lock()
		tree.Insert(art.Key(path), nil)
		mu.Unlock()

		return filepath.SkipDir
	})
}

func directoryContainsGitRepoMarker(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), ".git") {
			return true
		}
	}

	return false
}

func pathContainsGitRepoMarker(path string) (bool, error) {
	if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func GitObjectsDirs(gitRootPath string) ([]string, error) {
	dirs, err := os.ReadDir(filepath.Join(gitRootPath, ".git/objects"))
	if err != nil {
		return nil, err
	}

	var objectsDirs []string

	for _, dir := range dirs {
		if dir.IsDir() && (!strings.EqualFold(dir.Name(), "info") && !strings.EqualFold(dir.Name(), "pack")) {
			objectsDirs = append(objectsDirs, filepath.Join(gitRootPath, ".git/objects", dir.Name()))
		}
	}

	return objectsDirs, nil
}

func GitObjectsPackFiles(gitRootPath string) ([]string, error) {
	info, err := os.ReadDir(filepath.Join(gitRootPath, ".git/objects/pack"))
	if err != nil {
		return nil, err
	}

	var packFiles []string

	for _, i := range info {
		if !i.IsDir() && strings.HasSuffix(i.Name(), "pack") {
			packFiles = append(packFiles, filepath.Join(gitRootPath, ".git/objects/pack", i.Name()))
		}
	}

	return packFiles, nil
}

func GitObjects(gitRootPath string) ([]string, error) {
	var objects []string

	dirs, err := GitObjectsDirs(gitRootPath)
	if err != nil {
		return nil, err
	}

	objects = append(objects, dirs...)

	packs, err := GitObjectsPackFiles(gitRootPath)
	if err != nil {
		return nil, err
	}

	objects = append(objects, packs...)

	return objects, nil
}
