package cmd

import (
	"os"
	"time"

	"github.com/zbiljic/fget/pkg/vconfig"
)

const configStateVersionV2 = "2"

// cached during usage
var cacheConfigStateV2 *configStateV2

type configStateV2 struct {
	Version        string    `json:"version"`
	CreateTime     time.Time `json:"create_time"`
	UpdateTime     time.Time `json:"update_time"`
	Roots          []string  `json:"roots"`
	TotalCount     int       `json:"total_count"`
	RemainingCount int       `json:"remaining_count"`
	Paths          []string  `json:"paths"`
}

// newConfigStateV2 - new state config.
func newConfigStateV2() *configStateV2 {
	config := new(configStateV2)
	config.Version = configStateVersionV2
	config.CreateTime = time.Now()
	config.UpdateTime = time.Now()
	config.Roots = make([]string, 0)
	config.Paths = make([]string, 0)
	return config
}

// loadConfigStateV2 - loads a state config.
func loadConfigStateV2(baseDir, stateName string) (*configStateV2, error) {
	configStateMutex.RLock()
	defer configStateMutex.RUnlock()

	// if already cached, return the cached value
	if cacheConfigStateV2 != nil {
		return cacheConfigStateV2, nil
	}

	filename := configStateFilename(baseDir, stateName)

	config, err := vconfig.LoadConfig[configStateV2](filename)
	if err != nil {
		return nil, err
	}

	// cache config
	cacheConfigStateV2 = config

	// success
	return config, nil
}

// saveConfigStateV2 - saves state config.
func saveConfigStateV2(baseDir, stateName string, config *configStateV2) error {
	configStateMutex.Lock()
	defer configStateMutex.Unlock()

	config.UpdateTime = time.Now()
	config.RemainingCount = len(config.Paths)

	filename := configStateFilename(baseDir, stateName)

	if err := vconfig.SaveConfig(config, filename); err != nil {
		return err
	}

	// update the cache
	cacheConfigStateV2 = config

	return nil
}

func clearConfigStateV2(baseDir, stateName string) error {
	configStateMutex.Lock()
	defer configStateMutex.Unlock()

	// clear cached
	cacheConfigStateV2 = nil

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
