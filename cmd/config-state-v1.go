package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/go-git/go-billy/v5/osfs"
)

const (
	configStateVersion            = "1"
	configStateFileFormat         = ".fget.state-%s.json"
	configStateCheckpointInterval = 100
)

var (
	// cached during usage
	cacheConfigStateV1 *configStateV1
	// all access to state configuration file should be synchronized
	configStateMutex = &sync.RWMutex{}
)

type configStateV1 struct {
	Version    string   `json:"version"`
	Roots      []string `json:"roots"`
	Checkpoint string   `json:"checkpoint"`
}

// newConfigStateV1 - new state config.
func newConfigStateV1() *configStateV1 {
	config := new(configStateV1)
	config.Version = configStateVersion
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

	fs := osfs.New(baseDir)
	configStateFile := fmt.Sprintf(configStateFileFormat, stateName)

	if _, err := fs.Stat(configStateFile); err != nil {
		return nil, err
	}

	f, err := fs.Open(configStateFile)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	config := &configStateV1{}

	err = json.Unmarshal(data, config)
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

	data, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return err
	}

	// update the cache
	cacheConfigStateV1 = config

	fs := osfs.New(baseDir)
	configStateFile := fmt.Sprintf(configStateFileFormat, stateName)

	f, err := fs.Create(configStateFile)
	if err != nil {
		return err
	}

	n, err := f.Write(data)
	if err == nil && n < len(data) {
		return io.ErrShortWrite
	}

	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

func clearConfigStateV1(baseDir, stateName string) error {
	configStateMutex.Lock()
	defer configStateMutex.Unlock()

	// clear cached
	cacheConfigStateV1 = nil

	fs := osfs.New(baseDir)
	configStateFile := fmt.Sprintf(configStateFileFormat, stateName)

	if _, err := fs.Stat(configStateFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	if err := fs.Remove(configStateFile); err != nil {
		return err
	}

	return nil
}
