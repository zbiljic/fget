package fsfind

import (
	"os"
	"path/filepath"
	"strings"

	art "github.com/plar/go-adaptive-radix-tree"
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
