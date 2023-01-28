package cmd

import (
	"os"

	"github.com/zbiljic/fget/pkg/vconfig"
)

const configStateVersionV1 = "1"

// cached during usage
var cacheConfigStateV1 *configStateV1

type configStateV1 struct {
	Version    string   `json:"version"`
	Roots      []string `json:"roots"`
	Checkpoint string   `json:"checkpoint"`
}

// newConfigStateV1 - new state config.
func newConfigStateV1() *configStateV1 {
	config := new(configStateV1)
	config.Version = configStateVersionV1
	config.Roots = make([]string, 0)
	return config
}

// loadConfigStateV1 - loads a state config.
func loadConfigStateV1(baseDir, stateName string) (*configStateV1, error) {
	configStateMutex.RLock()
	defer configStateMutex.RUnlock()

	// if already cached, return the cached value
	if cacheConfigStateV1 != nil {
		return cacheConfigStateV1, nil
	}

	filename := configStateFilename(baseDir, stateName)

	config, err := vconfig.LoadConfig[configStateV1](filename)
	if err != nil {
		return nil, err
	}

	// cache config
	cacheConfigStateV1 = config

	// success
	return config, nil
}

// saveConfigStateV1 - saves state config.
func saveConfigStateV1(baseDir, stateName string, config *configStateV1) error {
	configStateMutex.Lock()
	defer configStateMutex.Unlock()

	filename := configStateFilename(baseDir, stateName)

	if err := vconfig.SaveConfig(config, filename); err != nil {
		return err
	}

	// update the cache
	cacheConfigStateV1 = config

	return nil
}

func clearConfigStateV1(baseDir, stateName string) error {
	configStateMutex.Lock()
	defer configStateMutex.Unlock()

	// clear cached
	cacheConfigStateV1 = nil

	filename := configStateFilename(baseDir, stateName)

	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	if err := os.Remove(filename); err != nil {
		return err
	}

	return nil
}
