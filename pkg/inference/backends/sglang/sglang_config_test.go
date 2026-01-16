package sglang

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

type mockModelBundle struct {
	safetensorsPath string
	runtimeConfig   *types.Config
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

func (m *mockModelBundle) RuntimeConfig() types.ModelConfig {
	if m.runtimeConfig == nil {
		return &types.Config{}
	}
	return m.runtimeConfig
}

func (m *mockModelBundle) DDUFPath() string {
	return ""
}

func (m *mockModelBundle) RootDir() string {
	return "/path/to/bundle"
}

func TestGetArgs(t *testing.T) {
	tests := []struct {
		name        string
		config      *inference.BackendConfiguration
		bundle      *mockModelBundle
		mode        inference.BackendMode
		expected    []string
		expectError bool
	}{
		{
			name: "empty safetensors path should error",
			bundle: &mockModelBundle{
				safetensorsPath: "",
			},
			mode:        inference.BackendModeCompletion,
			config:      nil,
			expected:    nil,
			expectError: true,
		},
		{
			name: "basic args without context size",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model/model.safetensors",
			},
			mode:   inference.BackendModeCompletion,
			config: nil,
			expected: []string{
				"-m",
				"sglang.launch_server",
				"--model-path",
				"/path/to/model",
				"--host",
				"127.0.0.1",
				"--port",
				"30000",
			},
		},
		{
			name: "with backend context size",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model/model.safetensors",
			},
			mode: inference.BackendModeCompletion,
			config: &inference.BackendConfiguration{
				ContextSize: int32ptr(8192),
			},
			expected: []string{
				"-m",
				"sglang.launch_server",
				"--model-path",
				"/path/to/model",
				"--host",
				"127.0.0.1",
				"--port",
				"30000",
				"--context-length",
				"8192",
			},
		},
		{
			name: "with model context size (takes precedence)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model/model.safetensors",
				runtimeConfig: &types.Config{
					ContextSize: int32ptr(16384),
				},
			},
			mode: inference.BackendModeCompletion,
			config: &inference.BackendConfiguration{
				ContextSize: int32ptr(8192),
			},
			expected: []string{
				"-m",
				"sglang.launch_server",
				"--model-path",
				"/path/to/model",
				"--host",
				"127.0.0.1",
				"--port",
				"30000",
				"--context-length",
				"16384",
			},
		},
		{
			name: "embedding mode",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model/model.safetensors",
			},
			mode:   inference.BackendModeEmbedding,
			config: nil,
			expected: []string{
				"-m",
				"sglang.launch_server",
				"--model-path",
				"/path/to/model",
				"--host",
				"127.0.0.1",
				"--port",
				"30000",
				"--is-embedding",
			},
		},
		{
			name: "reranking mode",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model/model.safetensors",
			},
			mode:   inference.BackendModeReranking,
			config: nil,
			expected: []string{
				"-m",
				"sglang.launch_server",
				"--model-path",
				"/path/to/model",
				"--host",
				"127.0.0.1",
				"--port",
				"30000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewDefaultSGLangConfig()
			args, err := config.GetArgs(tt.bundle, "127.0.0.1:30000", tt.mode, tt.config)

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

func TestGetContextLength(t *testing.T) {
	tests := []struct {
		name          string
		modelCfg      types.ModelConfig
		backendCfg    *inference.BackendConfiguration
		expectedValue *int32
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
			expectedValue: int32ptr(4096),
		},
		{
			name: "model config only",
			modelCfg: &types.Config{
				ContextSize: int32ptr(8192),
			},
			backendCfg:    nil,
			expectedValue: int32ptr(8192),
		},
		{
			name: "model config takes precedence",
			modelCfg: &types.Config{
				ContextSize: int32ptr(16384),
			},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: int32ptr(4096),
			},
			expectedValue: int32ptr(16384),
		},
		{
			name:     "zero context size in backend config returns nil",
			modelCfg: &types.Config{},
			backendCfg: &inference.BackendConfiguration{
				ContextSize: int32ptr(0),
			},
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContextLength(tt.modelCfg, tt.backendCfg)
			if (result == nil) != (tt.expectedValue == nil) {
				t.Errorf("expected nil=%v, got nil=%v", tt.expectedValue == nil, result == nil)
			} else if result != nil && *result != *tt.expectedValue {
				t.Errorf("expected %d, got %d", *tt.expectedValue, *result)
			}
		})
	}
}

func int32ptr(v int32) *int32 {
	return &v
}
