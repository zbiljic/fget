package cmd

import (
	"sync"
	"time"
)

// all changes that modify state should be synchronized
var updateMutex = &sync.RWMutex{}

type (
	ctxKeyDryRun                   struct{}
	ctxKeyOnlyUpdated              struct{}
	ctxKeyPrintProjectInfoHeaderFn struct{}
	ctxKeyIsUpdateMutexLocked      struct{}
	ctxKeyShouldUpdateMutexUnlock  struct{}
)

const (
	poolDefaultMaxWorkers  = 10
	poolDefaultMaxCapacity = 100
)

const (
	// Default retry configuration
	defaultRetryWaitMin        = time.Second
	defaultRetryWaitMax        = 10 * time.Second
	defaultRetryMaxElapsedTime = time.Minute
)
