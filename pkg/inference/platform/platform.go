package platform

import "runtime"

// SupportsVLLM returns true if vLLM is supported on the current platform.
func SupportsVLLM() bool {
	return runtime.GOOS == "linux"
}
