package fsfind

import (
	"fmt"
	"os"
	"path/filepath"
)

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
		return "", fmt.Errorf("path is not directory: %s", path)
	}

	return path, nil
}
