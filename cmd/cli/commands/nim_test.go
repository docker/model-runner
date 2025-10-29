package commands

import (
	"testing"
)

func TestIsNIMImage(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{
			name:     "NIM image with full path",
			model:    "nvcr.io/nim/google/gemma-3-1b-it:latest",
			expected: true,
		},
		{
			name:     "NIM image without tag",
			model:    "nvcr.io/nim/meta/llama-3.1-8b-instruct",
			expected: true,
		},
		{
			name:     "Regular Docker Hub image",
			model:    "docker.io/library/ubuntu:latest",
			expected: false,
		},
		{
			name:     "Regular image without registry",
			model:    "ubuntu:latest",
			expected: false,
		},
		{
			name:     "HuggingFace model",
			model:    "hf.co/TheBloke/Llama-2-7B-Chat-GGUF",
			expected: false,
		},
		{
			name:     "Local model path",
			model:    "./models/llama-2-7b.gguf",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNIMImage(tt.model)
			if result != tt.expected {
				t.Errorf("isNIMImage(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestNIMContainerName(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{
			name:     "NIM image with tag",
			model:    "nvcr.io/nim/google/gemma-3-1b-it:latest",
			expected: "docker-model-nim-google-gemma-3-1b-it",
		},
		{
			name:     "NIM image without tag",
			model:    "nvcr.io/nim/meta/llama-3.1-8b-instruct",
			expected: "docker-model-nim-meta-llama-3.1-8b-instruct",
		},
		{
			name:     "NIM image with version tag",
			model:    "nvcr.io/nim/nvidia/nemo:24.01",
			expected: "docker-model-nim-nvidia-nemo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nimContainerName(tt.model)
			if result != tt.expected {
				t.Errorf("nimContainerName(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}
