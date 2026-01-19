package commands

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBackendMode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    inference.BackendMode
		expectError bool
	}{
		{
			name:        "completion mode lowercase",
			input:       "completion",
			expected:    inference.BackendModeCompletion,
			expectError: false,
		},
		{
			name:        "completion mode uppercase",
			input:       "COMPLETION",
			expected:    inference.BackendModeCompletion,
			expectError: false,
		},
		{
			name:        "completion mode mixed case",
			input:       "Completion",
			expected:    inference.BackendModeCompletion,
			expectError: false,
		},
		{
			name:        "embedding mode",
			input:       "embedding",
			expected:    inference.BackendModeEmbedding,
			expectError: false,
		},
		{
			name:        "reranking mode",
			input:       "reranking",
			expected:    inference.BackendModeReranking,
			expectError: false,
		},
		{
			name:        "image-generation mode",
			input:       "image-generation",
			expected:    inference.BackendModeImageGeneration,
			expectError: false,
		},
		{
			name:        "invalid mode",
			input:       "invalid",
			expected:    inference.BackendModeCompletion, // default on error
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    inference.BackendModeCompletion, // default on error
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseBackendMode(tt.input)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
