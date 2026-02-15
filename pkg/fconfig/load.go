package fconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zbiljic/fget/pkg/vconfig"
)

type EffectiveConfig struct {
	Config
	Sources     []string
	CatalogPath string
}

func LoadEffectiveConfig(homeDir, cwd, xdgConfigHome string) (*EffectiveConfig, error) {
	baseConfigPath := ResolveBaseConfigPath(xdgConfigHome, homeDir)

	overlayFiles, err := DiscoverConfigFiles(homeDir, cwd)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(overlayFiles)+1)
	if fileExists(baseConfigPath) {
		files = append(files, baseConfigPath)
	}
	for _, overlayFile := range overlayFiles {
		if !contains(files, overlayFile) {
			files = append(files, overlayFile)
		}
	}

	effective := &EffectiveConfig{}
	seenRoots := make(map[string]struct{})

	for _, file := range files {
		cfg, err := vconfig.LoadConfig[Config](file)
		if err != nil {
			return nil, fmt.Errorf("load config %s: %w", file, err)
		}

		if cfg.Version != "" {
			effective.Version = cfg.Version
		}

		for _, root := range cfg.Roots {
			if _, ok := seenRoots[root]; ok {
				continue
			}
			effective.Roots = append(effective.Roots, root)
			seenRoots[root] = struct{}{}
		}

		if cfg.Catalog.Path != "" {
			effective.Catalog.Path = cfg.Catalog.Path
		}

		if cfg.Tags.Defaults != nil {
			effective.Tags.Defaults = cfg.Tags.Defaults
		}

		effective.Sources = append(effective.Sources, file)
	}

	if effective.Version == "" {
		effective.Version = ConfigVersionV1
	}

	if effective.Catalog.Path == "" {
		effective.CatalogPath = ResolveDefaultCatalogPath(xdgConfigHome, homeDir)
	} else {
		effective.CatalogPath = expandPath(effective.Catalog.Path, homeDir, cwd)
	}

	return effective, nil
}

func expandPath(path, homeDir, cwd string) string {
	switch {
	case path == "~":
		return homeDir
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(homeDir, path[2:])
	case filepath.IsAbs(path):
		return path
	default:
		return filepath.Join(cwd, path)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
