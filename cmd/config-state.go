package cmd

import (
	"fmt"
	"path/filepath"
	"sync"
)

const (
	configStateFileFormat         = ".fget.state-%s.json"
	configStateCheckpointInterval = 100
)

// all access to state configuration file should be synchronized
var configStateMutex = &sync.RWMutex{}

func configStateFilename(baseDir, stateName string) string {
	return filepath.Join(baseDir, fmt.Sprintf(configStateFileFormat, stateName))
}
