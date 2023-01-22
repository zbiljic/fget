package cmd

import (
	"sync"
	"time"
)

// all changes that modify state should be synchronized
var updateMutex = &sync.RWMutex{}

type (
	ctxKeyDryRun                   struct{}
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
	defaultRetryWaitMin        = 1 * time.Second
	defaultRetryWaitMax        = 30 * time.Second
	defaultRetryMaxElapsedTime = time.Minute
)
