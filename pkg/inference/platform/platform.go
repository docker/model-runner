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

// SupportsDiffusers returns true if diffusers is supported on the current platform.
// Diffusers is supported on Linux (for Docker/CUDA) and macOS (for MPS/Apple Silicon).
func SupportsDiffusers() bool {
	// return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	return runtime.GOOS == "linux" // Support for macOS disabled for now until we design a solution to distribute it via Docker Desktop.
}
