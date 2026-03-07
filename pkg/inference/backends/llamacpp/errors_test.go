package llamacpp

import (
	"testing"
)

func TestExtractLlamaCppError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Metal buffer allocation failure",
			input:    "ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB",
			expected: "not enough GPU memory to load the model (Metal)",
		},
		{
			name:     "cudaMalloc OOM",
			input:    "ggml_backend_cuda_buffer_type_alloc_buffer: allocating 12.50 MiB on device 1: cudaMalloc failed: out of memory",
			expected: "not enough GPU memory to load the model (CUDA)",
		},
		{
			name: "loading error",
			input: `common_init_from_params: failed to load model '/models/model.gguf'
main: exiting due to model loading error`,
			expected: "failed to load model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractLlamaCppError(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractLlamaCppError() = %q, want %q", result, tt.expected)
			}
		})
	}
}
