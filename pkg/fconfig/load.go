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
	Sources    []string
	LinkSource string
	ScopeOwner string
}

type configFileState struct {
	Path   string
	Config *Config
}

func LoadConfigFile(path, homeDir string) (*Config, error) {
	cfg, err := vconfig.LoadConfig[Config](path)
	if err != nil {
		return nil, err
	}
	if err := validateConfigVersion(cfg.Version); err != nil {
		return nil, err
	}

	resolved := *cfg
	if resolved.Version == "" {
		resolved.Version = ConfigVersionV1
	}

	baseDir := filepath.Dir(path)
	resolved.Roots = resolveConfigRoots(cfg.Roots, homeDir, baseDir)
	if cfg.Catalog.Path != "" {
		resolved.Catalog.Path = filepath.Clean(expandPathFromBase(cfg.Catalog.Path, homeDir, baseDir))
	}
	resolved.Catalog.Imports = resolveCatalogImports(cfg.Catalog.Imports, homeDir, baseDir)
	resolved.Link = resolveLinkConfig(cfg.Link, homeDir, baseDir)

	return &resolved, nil
}

func LoadEffectiveConfig(homeDir, cwd, xdgConfigHome string) (*EffectiveConfig, error) {
	baseConfigPath := ResolveBaseConfigPath(xdgConfigHome, homeDir)

	overlayFiles, err := DiscoverConfigFiles(cwd)
	if err != nil {
		return nil, err
	}

	states, err := loadConfigFiles(overlayFiles, homeDir)
	if err != nil {
		return nil, err
	}

	effective := &EffectiveConfig{}
	seenRoots := make(map[string]struct{})
	seenImports := make(map[string]struct{})
	mergeState := func(state configFileState) {
		cfg := state.Config
		if cfg == nil {
			return
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
		for _, importPath := range cfg.Catalog.Imports {
			if _, ok := seenImports[importPath]; ok {
				continue
			}
			effective.Catalog.Imports = append(effective.Catalog.Imports, importPath)
			seenImports[importPath] = struct{}{}
		}

		if cfg.Link != nil {
			effective.Link = copyLinkConfig(cfg.Link)
			effective.LinkSource = state.Path
		}

		effective.Sources = append(effective.Sources, state.Path)
	}

	scopeOwnerIndex := resolveScopeOwnerIndex(states)
	if scopeOwnerIndex >= 0 {
		effective.ScopeOwner = states[scopeOwnerIndex].Path
		for _, state := range states[scopeOwnerIndex:] {
			mergeState(state)
		}
	} else {
		if fileExists(baseConfigPath) {
			baseConfig, err := LoadConfigFile(baseConfigPath, homeDir)
			if err != nil {
				return nil, fmt.Errorf("load config %s: %w", baseConfigPath, err)
			}
			effective.ScopeOwner = baseConfigPath
			mergeState(configFileState{Path: baseConfigPath, Config: baseConfig})
		}
		for _, state := range states {
			mergeState(state)
		}
	}

	if effective.Version == "" {
		effective.Version = ConfigVersionV1
	}

	if effective.Catalog.Path == "" {
		effective.Catalog.Path = ResolveDefaultCatalogPath(xdgConfigHome, homeDir)
	}

	return effective, nil
}

func resolveLinkConfig(cfg *LinkConfig, homeDir, baseDir string) *LinkConfig {
	if cfg == nil {
		return nil
	}

	resolved := *cfg
	if resolved.Match == "" {
		resolved.Match = "any"
	}
	if resolved.Layout == "" {
		resolved.Layout = "repo-id"
	}
	if resolved.Root == "" {
		resolved.Root = "."
	}

	resolved.Root = expandPathFromBase(resolved.Root, homeDir, baseDir)
	if resolved.SourceRoot != "" {
		resolved.SourceRoot = expandPathFromBase(resolved.SourceRoot, homeDir, baseDir)
	}

	if resolved.Tags == nil {
		resolved.Tags = []string{}
	}

	return &resolved
}

func validateConfigVersion(version string) error {
	switch version {
	case "", ConfigVersionV1, ConfigVersionV2:
		return nil
	default:
		return fmt.Errorf("unsupported config version %q", version)
	}
}

func loadConfigFiles(paths []string, homeDir string) ([]configFileState, error) {
	states := make([]configFileState, 0, len(paths))
	for _, path := range paths {
		cfg, err := LoadConfigFile(path, homeDir)
		if err != nil {
			return nil, fmt.Errorf("load config %s: %w", path, err)
		}
		states = append(states, configFileState{
			Path:   path,
			Config: cfg,
		})
	}

	return states, nil
}

func resolveScopeOwnerIndex(states []configFileState) int {
	for i := len(states) - 1; i >= 0; i-- {
		if definesExplicitScope(states[i].Config) {
			return i
		}
	}

	return -1
}

func definesExplicitScope(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	return cfg.Version == ConfigVersionV2 && cfg.Catalog.Path != ""
}

func resolveConfigRoots(roots []string, homeDir, baseDir string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		out = append(out, filepath.Clean(expandPathFromBase(root, homeDir, baseDir)))
	}

	return out
}

func resolveCatalogImports(paths []string, homeDir, baseDir string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out = append(out, resolveCatalogImportPath(path, homeDir, baseDir))
	}

	return out
}

func resolveCatalogImportPath(path, homeDir, baseDir string) string {
	path = filepath.Clean(expandPathFromBase(path, homeDir, baseDir))
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return path
	default:
		return filepath.Join(path, configFilename)
	}
}

func copyLinkConfig(cfg *LinkConfig) *LinkConfig {
	if cfg == nil {
		return nil
	}

	out := *cfg
	if cfg.Tags != nil {
		out.Tags = append([]string{}, cfg.Tags...)
	}

	return &out
}

func expandPathFromBase(path, homeDir, baseDir string) string {
	switch {
	case path == "~":
		return homeDir
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(homeDir, path[2:])
	case filepath.IsAbs(path):
		return path
	default:
		return filepath.Join(baseDir, path)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
