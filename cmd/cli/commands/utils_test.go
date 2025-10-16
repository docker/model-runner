package commands

import (
	"testing"
)

func TestIsOllamaModel(t *testing.T) {
	testCases := []struct {
		name     string
		model    string
		expected bool
	}{
		{
			name:     "ollama library model",
			model:    "ollama.com/library/smollm:135m",
			expected: true,
		},
		{
			name:     "ollama user model",
			model:    "ollama.com/user/custom-model:latest",
			expected: true,
		},
		{
			name:     "ollama simple",
			model:    "ollama.com/model",
			expected: true,
		},
		{
			name:     "docker hub model",
			model:    "docker.io/library/llama:latest",
			expected: false,
		},
		{
			name:     "huggingface model",
			model:    "hf.co/TheBloke/Llama-2-7B-GGUF",
			expected: false,
		},
		{
			name:     "plain model name",
			model:    "llama2:7b",
			expected: false,
		},
		{
			name:     "empty string",
			model:    "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isOllamaModel(tc.model)
			if result != tc.expected {
				t.Errorf("isOllamaModel(%q) = %v, expected %v", tc.model, result, tc.expected)
			}
		})
	}
}
