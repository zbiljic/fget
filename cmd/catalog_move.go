package cmd

import (
	"os"

	"github.com/zbiljic/fget/pkg/fconfig"
)

func migrateCatalogsForRepoMove(move fconfig.RepoMove) error {
	runtimeCtx, err := loadConfigRuntimeContext()
	if err != nil {
		return err
	}

	return migrateCatalogsForRepoMoveInRuntimeContext(move, runtimeCtx)
}

func migrateCatalogsForRepoMoveInRuntimeContext(move fconfig.RepoMove, runtimeCtx configRuntimeContext) error {
	runtimeCtx.Cwd = move.NewPath

	config, err := fconfig.LoadEffectiveConfig(runtimeCtx.HomeDir, runtimeCtx.Cwd, runtimeCtx.XDGConfigHome)
	if err != nil {
		return err
	}

	if _, err := os.Stat(config.Catalog.Path); err != nil {
		if os.IsNotExist(err) && len(config.Catalog.Imports) == 0 {
			return nil
		}
		return err
	}

	set, err := loadCatalogSetForEffectiveConfig(config, runtimeCtx.HomeDir)
	if err != nil {
		return err
	}

	for i := range set.Sources {
		source := &set.Sources[i]
		if !source.Catalog.ApplyRepoMove(move) {
			continue
		}
		if err := fconfig.SaveCatalog(source.CatalogPath, source.Catalog); err != nil {
			return err
		}
	}

	return nil
}
