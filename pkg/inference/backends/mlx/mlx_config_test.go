package mlx

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

func TestGetMaxTokens(t *testing.T) {
	tests := []struct {
		name          string
		modelCfg      types.ModelConfig
		backendCfg    *inference.BackendConfiguration
		expectedValue *uint64
	}{
		{
			name:          "no config",
			modelCfg:      &types.Config{},
			backendCfg:    nil,
			expectedValue: nil,
		},
		{
			name:     "backend config only",
			modelCfg: &types.Config{},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: int32ptr(4096),
			},
			expectedValue: uint64ptr(4096),
		},
		{
			name: "model config only",
			modelCfg: &types.Config{
				ContextSize: int32ptr(8192),
			},
			backendCfg:    nil,
			expectedValue: uint64ptr(8192),
		},
		{
			name: "backend config takes precedence",
			modelCfg: &types.Config{
				ContextSize: int32ptr(16384),
			},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: int32ptr(4096),
			},
			expectedValue: uint64ptr(4096),
		},
		{
			name: "model config used as fallback",
			modelCfg: &types.Config{
				ContextSize: int32ptr(16384),
			},
			backendCfg:    nil,
			expectedValue: uint64ptr(16384),
		},
		{
			name:     "zero context size in backend config returns nil",
			modelCfg: &types.Config{},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: int32ptr(0),
			},
			expectedValue: nil,
		},
		{
			name:          "nil model config with backend config",
			modelCfg:      nil,
			backendCfg:    &inference.BackendConfiguration{ContextSize: int32ptr(4096)},
			expectedValue: uint64ptr(4096),
		},
		{
			name:          "nil model config without backend config",
			modelCfg:      nil,
			backendCfg:    nil,
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMaxTokens(tt.modelCfg, tt.backendCfg)
			if (result == nil) != (tt.expectedValue == nil) {
				t.Errorf("expected nil=%v, got nil=%v", tt.expectedValue == nil, result == nil)
			} else if result != nil && *result != *tt.expectedValue {
				t.Errorf("expected %d, got %d", *tt.expectedValue, *result)
			}
		})
	}
}

func int32ptr(n int32) *int32 {
	return &n
}

func uint64ptr(n uint64) *uint64 {
	return &n
}
