package diffusers

import (
	"testing"
)

func TestExtractPythonError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "custom diffusers error marker",
			input:    "DIFFUSERS_ERROR: No GPU found. A GPU is needed for quantization.",
			expected: "No GPU found. A GPU is needed for quantization.",
		},
		{
			name: "custom error marker in traceback",
			input: `Traceback (most recent call last):
  File "server.py", line 350, in main
    load_model(args.model_path)
RuntimeError: Failed to load DDUF file: No GPU found
DIFFUSERS_ERROR: No GPU found. A GPU is needed for quantization.`,
			expected: "No GPU found. A GPU is needed for quantization.",
		},
		{
			name: "python runtime error",
			input: `RuntimeError: Failed to load DDUF file: No GPU found. A GPU is needed for quantization.
RuntimeError: No GPU found. A GPU is needed for quantization.`,
			expected: "RuntimeError: Failed to load DDUF file: No GPU found. A GPU is needed for quantization.",
		},
		{
			name: "full python traceback",
			input: `    raise RuntimeError(f"Failed to load DDUF file: {e}")
RuntimeError: Failed to load DDUF file: No GPU found. A GPU is needed for quantization.
RuntimeError: No GPU found. A GPU is needed for quantization.

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "<frozen runpy>", line 198, in _run_module_as_main
  File "<frozen runpy>", line 88, in _run_code
  File "/opt/diffusers-env/lib/python3.12/site-packages/diffusers_server/server.py", line 358, in <module>
    main()
  File "/opt/diffusers-env/lib/python3.12/site-packages/diffusers_server/server.py", line 350, in main
    load_model(args.model_path)
  File "/opt/diffusers-env/lib/python3.12/site-packages/diffusers_server/server.py", line 139, in load_model
    pipeline = load_model_from_dduf(model_path, device, dtype)`,
			expected: "RuntimeError: Failed to load DDUF file: No GPU found. A GPU is needed for quantization.",
		},
		{
			name:     "GPU not found error",
			input:    "Some log output\nNo GPU found. A GPU is needed for quantization.\nMore logs",
			expected: "No GPU found.",
		},
		{
			name:     "CUDA out of memory error",
			input:    "CUDA out of memory. Tried to allocate 2.00 GiB",
			expected: "CUDA out of memory.",
		},
		{
			name:     "import error",
			input:    "ImportError: No module named 'torch'",
			expected: "ImportError: No module named 'torch'",
		},
		{
			name:     "module not found error",
			input:    "ModuleNotFoundError: No module named 'diffusers'",
			expected: "ModuleNotFoundError: No module named 'diffusers'",
		},
		{
			name:     "value error",
			input:    "ValueError: Invalid model path",
			expected: "ValueError: Invalid model path",
		},
		{
			name:     "short output without pattern",
			input:    "some random error",
			expected: "some random error",
		},
		{
			name:     "empty output",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPythonError(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractPythonError() = %q, want %q", result, tt.expected)
			}
		})
	}
}
