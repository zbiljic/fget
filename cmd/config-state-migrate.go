package cmd

import (
	"fmt"
	"os"

	"github.com/zbiljic/fget/pkg/vconfig"
)

var errConfigStateMigrateVersion = func(version string, err error) error {
	return fmt.Errorf("unable to load state config version '%s': %w", version, err)
}

func configStateLoadCreateMigrate(baseDir, stateName string, roots ...string) (*configStateV2, error) {
	filename := configStateFilename(baseDir, stateName)

	version, err := vconfig.GetVersion(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// fallback create new config
			return createConfigState(roots...)
		}

		return nil, err
	}

	switch version {
	case configStateVersionV1:
		currentConfig, err := loadConfigStateV1(baseDir, stateName)
		if err != nil {
			return nil, errConfigStateMigrateVersion(version, err)
		}

		newConfig := newConfigStateV2()
		// migrate
		newConfig.Roots = make([]string, len(currentConfig.Roots))
		copy(newConfig.Roots, currentConfig.Roots)

		if err := saveConfigStateV2(baseDir, stateName, newConfig); err != nil {
			return nil, err
		}

		return configStateLoadCreateMigrate(baseDir, stateName, roots...)
	case configStateVersionV2:
		currentConfig, err := loadConfigStateV2(baseDir, stateName)
		if err != nil {
			return nil, errConfigStateMigrateVersion(version, err)
		}
		return currentConfig, nil
	default:
		return nil, fmt.Errorf("unknown version: '%s'", version)
	}
}
