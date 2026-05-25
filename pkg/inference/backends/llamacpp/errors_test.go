package llamacpp

import (
	"strings"
	"testing"
)

func TestExtractLlamaCppError(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expected        string
		expectedPrefix  string
		expectTruncated bool
	}{
		{
			name:  "Metal buffer allocation failure",
			input: "ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB",
			expected: "not enough GPU memory to load the model (Metal)\n\nVerbose output:\n" +
				"ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB",
		},
		{
			name:  "cudaMalloc OOM",
			input: "ggml_backend_cuda_buffer_type_alloc_buffer: allocating 12.50 MiB on device 1: cudaMalloc failed: out of memory",
			expected: "not enough GPU memory to load the model (CUDA)\n\nVerbose output:\n" +
				"ggml_backend_cuda_buffer_type_alloc_buffer: allocating 12.50 MiB on device 1: cudaMalloc failed: out of memory",
		},
		{
			name: "loading error",
			input: `common_init_from_params: failed to load model '/models/model.gguf'
main: exiting due to model loading error`,
			expected: "failed to load model\n\nVerbose output:\n" +
				"common_init_from_params: failed to load model '/models/model.gguf'\n" +
				"main: exiting due to model loading error",
		},
		{
			name:  "input with leading/trailing whitespace",
			input: "\n\n  ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB  \n\n",
			expected: "not enough GPU memory to load the model (Metal)\n\nVerbose output:\n" +
				"ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB",
		},
		{
			name:  "truncation of large output",
			input: "ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB\n" + strings.Repeat("verbose log line\n", 500),
			expectedPrefix: "not enough GPU memory to load the model (Metal)\n\nVerbose output:\n" +
				"ggml_metal_buffer_init: error: failed to allocate buffer, size = 2048.00 MiB\n",
			expectTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractLlamaCppError(tt.input)
			if tt.expectTruncated {
				if !strings.HasPrefix(result, tt.expectedPrefix) {
					t.Errorf("ExtractLlamaCppError() = %q, want prefix %q", result, tt.expectedPrefix)
				}
				if !strings.HasSuffix(result, "...[truncated]") {
					t.Errorf("ExtractLlamaCppError() = %q, want suffix ...[truncated]", result)
				}
			} else if result != tt.expected {
				t.Errorf("ExtractLlamaCppError() = %q, want %q", result, tt.expected)
			}
		})
	}
}
