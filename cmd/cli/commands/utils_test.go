package commands

import (
	"errors"
	"fmt"
	"testing"
)

func TestStripDefaultsFromModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ai prefix and latest tag",
			input:    "ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "ai prefix with custom tag",
			input:    "ai/gemma3:v1",
			expected: "gemma3:v1",
		},
		{
			name:     "custom org with latest tag",
			input:    "myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
		{
			name:     "simple model name with latest",
			input:    "gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "simple model name without tag",
			input:    "gemma3",
			expected: "gemma3",
		},
		{
			name:     "ai prefix without tag",
			input:    "ai/gemma3",
			expected: "gemma3",
		},
		{
			name:     "huggingface model with latest",
			input:    "hf.co/bartowski/model:latest",
			expected: "hf.co/bartowski/model",
		},
		{
			name:     "huggingface model with custom tag",
			input:    "hf.co/bartowski/model:Q4_K_S",
			expected: "hf.co/bartowski/model:Q4_K_S",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "docker.io registry with ai prefix and latest tag",
			input:    "docker.io/ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "index.docker.io registry with ai prefix and latest tag",
			input:    "index.docker.io/ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "docker.io registry with ai prefix and custom tag",
			input:    "docker.io/ai/gemma3:v1",
			expected: "gemma3:v1",
		},
		{
			name:     "docker.io registry with custom org and latest tag",
			input:    "docker.io/myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
		{
			name:     "index.docker.io registry with custom org and latest tag",
			input:    "index.docker.io/myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripDefaultsFromModelName(tt.input)
			if result != tt.expected {
				t.Errorf("stripDefaultsFromModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestHandleClientErrorFormat verifies that the error format follows the expected pattern.
func TestHandleClientErrorFormat(t *testing.T) {
	t.Run("error format is message: original error", func(t *testing.T) {
		originalErr := fmt.Errorf("network timeout")
		message := "Failed to fetch data"

		result := handleClientError(originalErr, message)

		expected := fmt.Errorf("%s: %w", message, originalErr).Error()
		if result.Error() != expected {
			t.Errorf("Error format mismatch.\nExpected: %q\nGot: %q", expected, result.Error())
		}

		if !errors.Is(result, originalErr) {
			t.Error("Error wrapping is not preserved - errors.Is() check failed")
		}
	})
}
