package commands

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference/models"
)

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple model name",
			input:    "gemma3",
			expected: "ai/gemma3:latest",
		},
		{
			name:     "model name with tag",
			input:    "gemma3:v1",
			expected: "ai/gemma3:v1",
		},
		{
			name:     "model name with org",
			input:    "myorg/gemma3",
			expected: "myorg/gemma3:latest",
		},
		{
			name:     "model name with org and tag",
			input:    "myorg/gemma3:v1",
			expected: "myorg/gemma3:v1",
		},
		{
			name:     "fully qualified model name",
			input:    "ai/gemma3:latest",
			expected: "ai/gemma3:latest",
		},
		{
			name:     "huggingface model",
			input:    "hf.co/bartowski/model",
			expected: "hf.co/bartowski/model:latest",
		},
		{
			name:     "huggingface model with tag",
			input:    "hf.co/bartowski/model:Q4_K_S",
			expected: "hf.co/bartowski/model:q4_k_s",
		},
		{
			name:     "registry with model",
			input:    "docker.io/library/model",
			expected: "docker.io/library/model:latest",
		},
		{
			name:     "registry with model and tag",
			input:    "docker.io/library/model:v1",
			expected: "docker.io/library/model:v1",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "ai prefix already present",
			input:    "ai/gemma3",
			expected: "ai/gemma3:latest",
		},
		{
			name:     "model name with latest tag already",
			input:    "gemma3:latest",
			expected: "ai/gemma3:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.NormalizeModelName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

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

// TestNormalizeModelNameConsistency verifies that locally packaged models
// (without namespace) get normalized the same way as other operations.
// This test documents the fix for the bug where `docker model package my-model`
// would create a model that couldn't be run with `docker model run my-model`.
func TestNormalizeModelNameConsistency(t *testing.T) {
	tests := []struct {
		name                    string
		userProvidedName        string
		expectedNormalizedName  string
		description             string
	}{
		{
			name:                   "locally packaged model without namespace",
			userProvidedName:       "my-model",
			expectedNormalizedName: "ai/my-model:latest",
			description:            "When a user packages a local model as 'my-model', it should be normalized to 'ai/my-model:latest'",
		},
		{
			name:                   "locally packaged model without namespace but with tag",
			userProvidedName:       "my-model:v1.0",
			expectedNormalizedName: "ai/my-model:v1.0",
			description:            "When a user packages a local model as 'my-model:v1.0', it should be normalized to 'ai/my-model:v1.0'",
		},
		{
			name:                   "model with explicit namespace",
			userProvidedName:       "myorg/my-model",
			expectedNormalizedName: "myorg/my-model:latest",
			description:            "When a user packages a model with explicit org 'myorg/my-model', it should keep the org",
		},
		{
			name:                   "model with ai namespace explicitly set",
			userProvidedName:       "ai/my-model",
			expectedNormalizedName: "ai/my-model:latest",
			description:            "When a user explicitly sets 'ai/' namespace, it should remain the same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.NormalizeModelName(tt.userProvidedName)
			if result != tt.expectedNormalizedName {
				t.Errorf("%s: NormalizeModelName(%q) = %q, want %q", 
					tt.description, tt.userProvidedName, result, tt.expectedNormalizedName)
			}
		})
	}
}
