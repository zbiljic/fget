package cmd

import "fmt"

func createConfigState(roots ...string) (*configStateV2, error) {
	config := newConfigStateV2()
	config.Roots = append(config.Roots, roots...)
	return config, nil
}

func loadOrCreateConfigState(baseDir, stateName string, roots ...string) (*configStateV2, error) {
	return configStateLoadCreateMigrate(baseDir, stateName, roots...)
}

func saveConfigState(baseDir, stateName string, config *configStateV2) error {
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
