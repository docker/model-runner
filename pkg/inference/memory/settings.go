package memory

import "sync/atomic"

var runtimeMemoryCheck atomic.Bool

func SetRuntimeMemoryCheck(enabled bool) {
	runtimeMemoryCheck.Store(enabled)
}

func RuntimeMemoryCheckEnabled() bool {
	return runtimeMemoryCheck.Load()
}

var runtimeLoaderMemoryCheck atomic.Bool

func init() {
	// Loader memory checks are enabled by default to prevent OOM errors.
	runtimeLoaderMemoryCheck.Store(true)
}

func SetRuntimeLoaderMemoryCheck(enabled bool) {
	runtimeLoaderMemoryCheck.Store(enabled)
}

func RuntimeLoaderMemoryCheckEnabled() bool {
	return runtimeLoaderMemoryCheck.Load()
}
