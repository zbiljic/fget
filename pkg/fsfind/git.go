package fsfind

import (
	"os"
	"path/filepath"
	"strings"

	art "github.com/plar/go-adaptive-radix-tree/v2"
)

func GitDirectoriesTree(paths ...string) (art.Tree, error) {
	tree := art.New()

	dirWalkerFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		dir := filepath.Dir(path)
		key := art.Key(dir)

		if info.IsDir() && strings.EqualFold(".git", info.Name()) {
			tree.Insert(key, nil)
			return filepath.SkipDir
		}

		if _, ok := tree.Search(key); ok {
			return filepath.SkipDir
		}

		return nil
	}

	for _, rootPath := range paths {
		err := filepath.Walk(rootPath, dirWalkerFn)
		if err != nil {
			return nil, err
		}
	}

	return tree, nil
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
