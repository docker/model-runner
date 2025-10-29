package memory

import "sync"

var runtimeMemoryCheck bool
var runtimeMemoryCheckLock sync.Mutex

func SetRuntimeMemoryCheck(enabled bool) {
	runtimeMemoryCheckLock.Lock()
	defer runtimeMemoryCheckLock.Unlock()
	runtimeMemoryCheck = enabled
}

func RuntimeMemoryCheckEnabled() bool {
	runtimeMemoryCheckLock.Lock()
	defer runtimeMemoryCheckLock.Unlock()
	return runtimeMemoryCheck
}
