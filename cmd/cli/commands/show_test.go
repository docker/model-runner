package commands

import (
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
)

func TestFormatModelInfo(t *testing.T) {
	contextSize := int32(4096)
	tests := []struct {
		name     string
		model    dmrm.Model
		contains []string
	}{
		{
			name: "basic model info",
			model: dmrm.Model{
				ID:      "sha256:abc123",
				Tags:    []string{"ai/gemma3:latest", "mymodel:v1"},
				Created: 1704067200, // 2024-01-01 00:00:00 UTC
				Config: &types.Config{
					Format:       "gguf",
					Architecture: "llama",
					Parameters:   "7B",
					Size:         "4.5GB",
					Quantization: "Q4_K_M",
					ContextSize:  &contextSize,
				},
			},
			contains: []string{
				"Model:       sha256:abc123",
				"Tags:        ai/gemma3:latest, mymodel:v1",
				"Format:       gguf",
				"Architecture: llama",
				"Parameters:   7B",
				"Size:         4.5GB",
				"Quantization: Q4_K_M",
				"Context Size: 4096",
			},
		},
		{
			name: "model with GGUF metadata",
			model: dmrm.Model{
				ID:      "sha256:def456",
				Tags:    []string{"test:latest"},
				Created: 1704067200,
				Config: &types.Config{
					Format: "gguf",
					GGUF: map[string]string{
						"version":     "3",
						"tensor_type": "F16",
					},
				},
			},
			contains: []string{
				"Model:       sha256:def456",
				"GGUF Metadata:",
				"version: 3",
				"tensor_type: F16",
			},
		},
		{
			name: "model with no config",
			model: dmrm.Model{
				ID:      "sha256:noconfig",
				Tags:    []string{"empty:latest"},
				Created: 1704067200,
				Config:  nil,
			},
			contains: []string{
				"Model:       sha256:noconfig",
				"Tags:        empty:latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatModelInfo(tt.model)
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("formatModelInfo() output missing expected string %q\nGot:\n%s", expected, output)
				}
			}
		})
	}
}
