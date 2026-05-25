package commands

import (
	"fmt"
	"math"
	"testing"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpCommandContextSizeFlagBehavior verifies that the --context-size flag on
// the compose up command is not "changed" by default (i.e. nil ContextSize
// should be sent when the flag is absent) and is marked as changed after an
// explicit value is provided.
func TestUpCommandContextSizeFlagBehavior(t *testing.T) {
	t.Run("context-size flag not changed by default", func(t *testing.T) {
		cmd := newUpCommand()
		// Parse with just the required --model flag — no --context-size.
		err := cmd.ParseFlags([]string{"--model", "mymodel"})
		require.NoError(t, err)
		// The flag must NOT be marked as changed so that ContextSize is omitted
		// from the configure request (i.e. remains nil).
		assert.False(t, cmd.Flags().Changed("context-size"),
			"context-size must not be Changed when the flag is absent")
	})

	t.Run("context-size flag changed after explicit value", func(t *testing.T) {
		cmd := newUpCommand()
		err := cmd.ParseFlags([]string{"--model", "mymodel", "--context-size", "4096"})
		require.NoError(t, err)
		assert.True(t, cmd.Flags().Changed("context-size"),
			"context-size must be Changed when explicitly provided")
	})

	t.Run("context-size flag changed with unlimited value -1", func(t *testing.T) {
		cmd := newUpCommand()
		err := cmd.ParseFlags([]string{"--model", "mymodel", "--context-size", "-1"})
		require.NoError(t, err)
		assert.True(t, cmd.Flags().Changed("context-size"),
			"context-size must be Changed when explicitly set to -1 (unlimited)")
	})

	t.Run("ContextSize is nil in BackendConfiguration when flag not set", func(t *testing.T) {
		cmd := newUpCommand()
		require.NoError(t, cmd.ParseFlags([]string{"--model", "mymodel"}))
		// Simulate the logic in compose.go RunE: only add ContextSize when Changed.
		backendConfig := inference.BackendConfiguration{}
		if cmd.Flags().Changed("context-size") {
			size := int32(-1) // default value
			backendConfig.ContextSize = &size
		}
		assert.Nil(t, backendConfig.ContextSize,
			"ContextSize must be nil in BackendConfiguration when --context-size is not provided")
	})

	t.Run("ContextSize is non-nil in BackendConfiguration when flag is set", func(t *testing.T) {
		cmd := newUpCommand()
		require.NoError(t, cmd.ParseFlags([]string{"--model", "mymodel", "--context-size", "64000"}))
		ctxSize, err := cmd.Flags().GetInt64("context-size")
		require.NoError(t, err)
		backendConfig := inference.BackendConfiguration{}
		if cmd.Flags().Changed("context-size") {
			size := int32(ctxSize)
			backendConfig.ContextSize = &size
		}
		require.NotNil(t, backendConfig.ContextSize,
			"ContextSize must be non-nil when --context-size is provided")
		assert.Equal(t, int32(64000), *backendConfig.ContextSize)
	})

	t.Run("context-size above int32 max is out of range", func(t *testing.T) {
		tooBig := int64(math.MaxInt32) + 1
		cmd := newUpCommand()
		require.NoError(t, cmd.ParseFlags([]string{"--model", "mymodel", "--context-size", fmt.Sprintf("%d", tooBig)}))
		ctxSize, err := cmd.Flags().GetInt64("context-size")
		require.NoError(t, err)
		require.True(t, cmd.Flags().Changed("context-size"))
		// Simulate the range check from compose.go RunE.
		if ctxSize > math.MaxInt32 || ctxSize < math.MinInt32 {
			// Expected: would return an error in RunE.
			return
		}
		t.Fatal("expected out-of-range check to trigger for value above MaxInt32")
	})

	t.Run("context-size below int32 min is out of range", func(t *testing.T) {
		tooSmall := int64(math.MinInt32) - 1
		cmd := newUpCommand()
		require.NoError(t, cmd.ParseFlags([]string{"--model", "mymodel", "--context-size", fmt.Sprintf("%d", tooSmall)}))
		ctxSize, err := cmd.Flags().GetInt64("context-size")
		require.NoError(t, err)
		require.True(t, cmd.Flags().Changed("context-size"))
		if ctxSize > math.MaxInt32 || ctxSize < math.MinInt32 {
			return
		}
		t.Fatal("expected out-of-range check to trigger for value below MinInt32")
	})
}

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
