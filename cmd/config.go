package cmd

import (
	"errors"
	"fmt"
	"io/fs"
)

func loadOrCreateConfigState(baseDir, stateName string, roots ...string) (*configStateV1, error) {
	// try to read existing
	config, err := loadConfigStateV1(baseDir, stateName)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		// create new
		config = newConfigStateV1()
		config.Roots = append(config.Roots, roots...)
	}

	return config, nil
}

func saveCheckpointConfigState(baseDir, stateName string, config *configStateV1, index int) error {
	if index%configStateCheckpointInterval == 0 {
		if err := saveConfigStateV1(baseDir, stateName, config); err != nil {
			return fmt.Errorf("save checkpoint config state: %w", err)
		}
	}

	return nil
}

func finishConfigState(baseDir, stateName string, config *configStateV1) error {
	if config.Checkpoint != "" {
		if err := saveConfigStateV1(baseDir, stateName, config); err != nil {
			return fmt.Errorf("finish save config state: %w", err)
		}
	} else {
		if err := clearConfigStateV1(baseDir, stateName); err != nil {
			return fmt.Errorf("finish clear config state: %w", err)
		}
	}

	return nil
}
