package cmd

import (
	"fmt"
	"sort"

	"golang.org/x/exp/slices"
)

func createConfigState(roots ...string) (*configStateV2, error) {
	config := newConfigStateV2()
	config.Roots = append(config.Roots, roots...)
	return config, nil
}

func loadOrCreateConfigState(baseDir, stateName string, roots ...string) (*configStateV2, error) {
	return configStateLoadCreateMigrate(baseDir, stateName, roots...)
}

func saveConfigState(baseDir, stateName string, config *configStateV2) error {
	// skip saving config state for single repository
	if len(config.Roots) == len(config.Paths) {
		// fast path
		if slices.Equal(config.Roots, config.Paths) {
			return nil
		}

		// slow path
		roots := make([]string, len(config.Roots))
		paths := make([]string, len(config.Paths))

		copy(roots, config.Roots)
		copy(paths, config.Paths)

		sort.Strings(roots)
		sort.Strings(paths)

		if slices.Equal(roots, paths) {
			return nil
		}
	}

	if err := saveConfigStateV2(baseDir, stateName, config); err != nil {
		return fmt.Errorf("save config state: %w", err)
	}
	return nil
}

func saveCheckpointConfigState(baseDir, stateName string, config *configStateV2, index int) error {
	if index%configStateCheckpointInterval == 0 {
		if err := saveConfigState(baseDir, stateName, config); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
	}

	return nil
}

func finishConfigState(baseDir, stateName string, config *configStateV2) error {
	if len(config.Paths) > 0 {
		if err := saveConfigState(baseDir, stateName, config); err != nil {
			return fmt.Errorf("finish: %w", err)
		}
	} else {
		if err := clearConfigStateV2(baseDir, stateName); err != nil {
			return fmt.Errorf("finish clear config state: %w", err)
		}
	}

	return nil
}
