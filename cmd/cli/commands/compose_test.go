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

func TestParseThinkToReasoningBudget(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *int32
		expectError bool
	}{
		{
			name:        "empty string returns nil",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "true returns nil (use server default)",
			input:       "true",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "TRUE returns nil (case insensitive)",
			input:       "TRUE",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "false disables reasoning",
			input:       "false",
			expected:    ptr(reasoningBudgetDisabled),
			expectError: false,
		},
		{
			name:        "high explicitly sets unlimited (-1)",
			input:       "high",
			expected:    ptr(reasoningBudgetUnlimited),
			expectError: false,
		},
		{
			name:        "medium sets 1024 tokens",
			input:       "medium",
			expected:    ptr(reasoningBudgetMedium),
			expectError: false,
		},
		{
			name:        "low sets 256 tokens",
			input:       "low",
			expected:    ptr(reasoningBudgetLow),
			expectError: false,
		},
		{
			name:        "invalid value returns error",
			input:       "invalid",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "numeric string returns error",
			input:       "1024",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseThinkToReasoningBudget(tt.input)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expected == nil {
					assert.Nil(t, result)
				} else {
					require.NotNil(t, result)
					assert.Equal(t, *tt.expected, *result)
				}
			}
		})
	}
}
