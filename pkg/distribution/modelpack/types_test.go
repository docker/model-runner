package modelpack

import (
	"testing"
)

// TestIsModelPackConfig tests detecting ModelPack format from raw config bytes
func TestIsModelPackConfig(t *testing.T) {
	// Prepare test ModelPack format config (has paramSize field)
	modelPackConfig := `{
		"descriptor": {"createdAt": "2025-01-15T10:30:00Z"},
		"config": {"paramSize": "8B", "format": "gguf"}
	}`

	// Docker format config (uses parameters instead of paramSize)
	dockerConfig := `{
		"config": {"parameters": "8B", "format": "gguf"},
		"descriptor": {"created": "2025-01-15T10:30:00Z"}
	}`

	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "ModelPack config with paramSize",
			input:    []byte(modelPackConfig),
			expected: true,
		},
		{
			name:     "Docker config with parameters",
			input:    []byte(dockerConfig),
			expected: false,
		},
		{
			name:     "empty JSON object",
			input:    []byte("{}"),
			expected: false,
		},
		{
			name:     "invalid JSON",
			input:    []byte("not json"),
			expected: false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: false,
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: false,
		},
		{
			name:     "config with createdAt field",
			input:    []byte(`{"descriptor": {"createdAt": "2025-01-01T00:00:00Z"}}`),
			expected: true,
		},
		{
			name:     "config with modelfs field",
			input:    []byte(`{"modelfs": {"type": "layers", "diffIds": []}}`),
			expected: true,
		},
		{
			name:     "false positive prevention - paramSize as value",
			input:    []byte(`{"config": {"description": "paramSize is 8B"}}`),
			expected: false,
		},
		{
			name:     "false positive prevention - createdAt as value",
			input:    []byte(`{"descriptor": {"note": "createdAt was yesterday"}}`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsModelPackConfig(tt.input)
			if got != tt.expected {
				t.Errorf("IsModelPackConfig() = %v, want %v", got, tt.expected)
			}
		})
	}
}
