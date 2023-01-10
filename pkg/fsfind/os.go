package fsfind

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrNotDirectory = errors.New("not directory")

func DirAbsPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if !fileInfo.IsDir() {
		return "", ErrNotDirectory
	}

	return path, nil
}
