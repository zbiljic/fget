package cmd

import "sync"

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
