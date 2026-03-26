package fconfig

import (
	"os"
	"path/filepath"
)

func DiscoverConfigFiles(cwd string) ([]string, error) {
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for path := cwdAbs; ; path = filepath.Dir(path) {
		filename := filepath.Join(path, configFilename)

		info, err := os.Stat(filename)
		if err == nil && !info.IsDir() {
			files = append(files, filename)
		}

		parent := filepath.Dir(path)
		if parent == path {
			break
		}
	}

	reverse(files)
	return files, nil
}

func ResolveBaseConfigPath(xdgConfigHome, homeDir string) string {
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, configDirname, configFilename)
	}

	return filepath.Join(homeDir, defaultConfigDir, configDirname, configFilename)
}

func ResolveDefaultCatalogPath(xdgConfigHome, homeDir string) string {
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, configDirname, catalogFilename)
	}

	return filepath.Join(homeDir, defaultConfigDir, configDirname, catalogFilename)
}

func reverse(values []string) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}
