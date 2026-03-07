package llamacpp

import "regexp"

// llamaCppErrorPatterns contains regex patterns to extract meaningful error messages
// from llama.cpp stderr output. The patterns are tried in order, and the first match wins.
var llamaCppErrorPatterns = []struct {
	pattern *regexp.Regexp
	message string
}{
	// Metal buffer allocation failure
	// https://github.com/ggml-org/llama.cpp/blob/ecd99d6a9acbc436bad085783bcd5d0b9ae9e9e9/ggml/src/ggml-metal/ggml-metal-device.m#L1498
	{regexp.MustCompile(`failed to allocate buffer, size = .*MiB`), "not enough GPU memory to load the model (Metal)"},
	// CUDA out of memory
	// https://github.com/ggml-org/llama.cpp/blob/ecd99d6a9acbc436bad085783bcd5d0b9ae9e9e9/ggml/src/ggml-cuda/ggml-cuda.cu#L710
	{regexp.MustCompile(`cudaMalloc failed: out of memory`), "not enough GPU memory to load the model (CUDA)"},
	// Generic model loading failure
	// https://github.com/ggml-org/llama.cpp/blob/ecd99d6a9acbc436bad085783bcd5d0b9ae9e9e9/tools/server/server.cpp#L254
	{regexp.MustCompile(`exiting due to model loading error`), "failed to load model"},
}

// ExtractLlamaCppError attempts to extract a meaningful error message from llama.cpp output.
// It looks for common error patterns and returns a cleaner, more user-friendly message.
// If no recognizable pattern is found, it returns the full output.
func ExtractLlamaCppError(output string) string {
	for _, entry := range llamaCppErrorPatterns {
		if entry.pattern.MatchString(output) {
			return entry.message
		}
	}
	return output
}
