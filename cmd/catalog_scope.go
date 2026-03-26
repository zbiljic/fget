package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zbiljic/fget/pkg/fconfig"
)

type catalogSource struct {
	ScopePath   string
	CatalogPath string
	Catalog     *fconfig.Catalog
	Writable    bool
}

type catalogSet struct {
	Sources []catalogSource
	View    *fconfig.Catalog
}

func loadCatalogSetForCurrentRuntimeContext() (*catalogSet, error) {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return nil, err
	}

	return loadCatalogSetForRuntimeContext(runtimeCtx)
}

func loadCatalogSetForRuntimeContext(runtimeCtx configRuntimeContext) (*catalogSet, error) {
	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return nil, err
	}

	set, err := loadCatalogSetForEffectiveConfig(config, runtimeCtx.HomeDir)
	if err != nil {
		return nil, err
	}

	return set, nil
}

func loadCatalogSetForEffectiveConfig(config *fconfig.EffectiveConfig, homeDir string) (*catalogSet, error) {
	if config == nil {
		return nil, errors.New("nil effective config")
	}

	set := &catalogSet{}
	seenCatalogs := make(map[string]int)
	seenScopes := make(map[string]struct{})

	addCatalog := func(scopePath, catalogPath string, writable bool) error {
		catalogPath = filepath.Clean(catalogPath)
		if index, ok := seenCatalogs[catalogPath]; ok {
			if writable {
				set.Sources[index].Writable = true
			}
			return nil
		}

		scopeRoot := ""
		if scopePath != "" {
			scopeRoot = filepath.Dir(scopePath)
		}

		catalog, err := loadExistingCatalog(catalogPath, scopeRoot)
		if err != nil {
			return err
		}

		set.Sources = append(set.Sources, catalogSource{
			ScopePath:   scopePath,
			CatalogPath: catalogPath,
			Catalog:     catalog,
			Writable:    writable,
		})
		seenCatalogs[catalogPath] = len(set.Sources) - 1

		return nil
	}

	var addImportedScope func(scopePath string) error
	addImportedScope = func(scopePath string) error {
		scopePath = filepath.Clean(scopePath)
		if _, ok := seenScopes[scopePath]; ok {
			return nil
		}
		seenScopes[scopePath] = struct{}{}

		cfg, err := fconfig.LoadConfigFile(scopePath, homeDir)
		if err != nil {
			return fmt.Errorf("load imported config %s: %w", scopePath, err)
		}
		if cfg.Catalog.Path == "" {
			return fmt.Errorf("imported config %s does not define catalog.path", scopePath)
		}

		if err := addCatalog(scopePath, cfg.Catalog.Path, false); err != nil {
			return err
		}
		for _, importPath := range cfg.Catalog.Imports {
			if err := addImportedScope(importPath); err != nil {
				return err
			}
		}

		return nil
	}

	if err := addCatalog(config.ScopeOwner, config.Catalog.Path, true); err != nil {
		return nil, err
	}
	if config.ScopeOwner != "" {
		seenScopes[filepath.Clean(config.ScopeOwner)] = struct{}{}
	}
	for _, importPath := range config.Catalog.Imports {
		if err := addImportedScope(importPath); err != nil {
			return nil, err
		}
	}

	catalogs := make([]*fconfig.Catalog, 0, len(set.Sources))
	for _, source := range set.Sources {
		catalogs = append(catalogs, source.Catalog)
	}
	set.View = fconfig.MergeCatalogs(catalogs...)

	return set, nil
}

func loadExistingCatalog(path, scopeRoot string) (*fconfig.Catalog, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("catalog does not exist at %s; run `fget catalog sync` first", path)
		}
		return nil, err
	}

	return fconfig.LoadCatalogWithScope(path, scopeRoot)
}

func (set *catalogSet) resolveWritableSource(selector string) (*catalogSource, error) {
	if set == nil {
		return nil, errors.New("nil catalog set")
	}

	matches := make([]int, 0, len(set.Sources))
	writableMatches := make([]int, 0, len(set.Sources))
	for i := range set.Sources {
		if _, err := fconfig.ResolveRepoIndex(set.Sources[i].Catalog, selector); err != nil {
			continue
		}

		matches = append(matches, i)
		if set.Sources[i].Writable {
			writableMatches = append(writableMatches, i)
		}
	}

	switch {
	case len(writableMatches) == 1:
		return &set.Sources[writableMatches[0]], nil
	case len(writableMatches) > 1:
		return nil, fmt.Errorf("repository %q is ambiguous across writable catalogs", selector)
	case len(matches) == 1:
		return &set.Sources[matches[0]], nil
	case len(matches) > 1:
		return nil, fmt.Errorf("repository %q is ambiguous across imported catalogs", selector)
	default:
		return nil, fmt.Errorf("repository %q not found in catalog", selector)
	}
}
