package cmd

import "sync"

// all changes that modify state should be synchronized
var updateMutex = &sync.RWMutex{}

type (
	ctxKeyDryRun struct{}
)
