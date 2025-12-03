package platform

import "runtime"

// SupportsVLLM returns true if vLLM is supported on the current platform.
func SupportsVLLM() bool {
	return runtime.GOOS == "linux"
}

// SupportsMLX returns true if MLX is supported on the current platform.
// MLX is only supported on macOS with ARM64 architecture (Apple Silicon).
func SupportsMLX() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

// SupportsSGLang returns true if SGLang is supported on the current platform.
func SupportsSGLang() bool {
	return runtime.GOOS == "linux"
}
