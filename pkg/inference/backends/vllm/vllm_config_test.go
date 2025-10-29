package vllm

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

type mockModelBundle struct {
	safetensorsPath string
	runtimeConfig   types.Config
}

func (m *mockModelBundle) GGUFPath() string {
	return ""
}

func (m *mockModelBundle) SafetensorsPath() string {
	return m.safetensorsPath
}

func (m *mockModelBundle) ChatTemplatePath() string {
	return ""
}

func (m *mockModelBundle) MMPROJPath() string {
	return ""
}

func (m *mockModelBundle) RuntimeConfig() types.Config {
	return m.runtimeConfig
}

func (m *mockModelBundle) RootDir() string {
	return "/path/to/bundle"
}

func TestGetArgs(t *testing.T) {
	tests := []struct {
		name        string
		config      *inference.BackendConfiguration
		bundle      *mockModelBundle
		expected    []string
		expectError bool
	}{
		{
			name: "empty safetensors path should error",
			bundle: &mockModelBundle{
				safetensorsPath: "",
			},
			config:      nil,
			expected:    nil,
			expectError: true,
		},
		{
			name: "basic args without context size",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: nil,
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
			},
		},
		{
			name: "with backend context size",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				ContextSize: 8192,
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--max-model-len",
				"8192",
			},
		},
		{
			name: "with runtime flags",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				RuntimeFlags: []string{"--gpu-memory-utilization", "0.9"},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--gpu-memory-utilization",
				"0.9",
			},
		},
		{
			name: "with model context size (takes precedence)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
				runtimeConfig: types.Config{
					ContextSize: ptrUint64(16384),
				},
			},
			config: &inference.BackendConfiguration{
				ContextSize: 8192,
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--max-model-len",
				"16384",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewDefaultVLLMConfig()
			args, err := config.GetArgs(tt.bundle, "/tmp/socket", inference.BackendModeCompletion, tt.config)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(args) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d\nexpected: %v\ngot: %v", len(tt.expected), len(args), tt.expected, args)
			}

			for i, arg := range args {
				if arg != tt.expected[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expected[i], arg)
				}
			}
		})
	}
}

func TestGetMaxModelLen(t *testing.T) {
	tests := []struct {
		name          string
		modelCfg      types.Config
		backendCfg    *inference.BackendConfiguration
		expectedValue *uint64
	}{
		{
			name:          "no config",
			modelCfg:      types.Config{},
			backendCfg:    nil,
			expectedValue: nil,
		},
		{
			name:     "backend config only",
			modelCfg: types.Config{},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: 4096,
			},
			expectedValue: ptrUint64(4096),
		},
		{
			name: "model config only",
			modelCfg: types.Config{
				ContextSize: ptrUint64(8192),
			},
			backendCfg:    nil,
			expectedValue: ptrUint64(8192),
		},
		{
			name: "model config takes precedence",
			modelCfg: types.Config{
				ContextSize: ptrUint64(16384),
			},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: 4096,
			},
			expectedValue: ptrUint64(16384),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMaxModelLen(tt.modelCfg, tt.backendCfg)
			if (result == nil) != (tt.expectedValue == nil) {
				t.Errorf("expected nil=%v, got nil=%v", tt.expectedValue == nil, result == nil)
			} else if result != nil && *result != *tt.expectedValue {
				t.Errorf("expected %d, got %d", *tt.expectedValue, *result)
			}
		})
	}
}

func ptrUint64(v uint64) *uint64 {
	return &v
}
