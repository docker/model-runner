package vllm

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
		return nil
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
				ContextSize: int32ptr(8192),
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
				runtimeConfig: &types.Config{
					ContextSize: int32ptr(16384),
				},
			},
			config: &inference.BackendConfiguration{
				ContextSize: int32ptr(8192),
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
		{
			name: "with simple HFOverrides",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					HFOverrides: inference.HFOverrides{
						"architectures": []interface{}{"Qwen3ForSequenceClassification"},
					},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--hf-overrides",
				`{"architectures":["Qwen3ForSequenceClassification"]}`,
			},
		},
		{
			name: "with complex HFOverrides (boolean and number)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					HFOverrides: inference.HFOverrides{
						"is_original_qwen3_reranker": true,
						"num_labels":                 float64(2),
					},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--hf-overrides",
				`{"is_original_qwen3_reranker":true,"num_labels":2}`,
			},
			expectError: false,
		},
		{
			name: "with nil VLLM config (no hf-overrides)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: nil,
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
			},
		},
		{
			name: "with empty HFOverrides (no hf-overrides)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					HFOverrides: inference.HFOverrides{},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
			},
		},
		{
			name: "with HFOverrides and context size",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				ContextSize: int32ptr(4096),
				VLLM: &inference.VLLMConfig{
					HFOverrides: inference.HFOverrides{
						"model_type": "bert",
					},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--max-model-len",
				"4096",
				"--hf-overrides",
				`{"model_type":"bert"}`,
			},
		},
		{
			name: "with GPU memory utilization 0.5",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(0.5),
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--gpu-memory-utilization",
				"0.5",
			},
		},
		{
			name: "with GPU memory utilization 0.0 (edge case)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(0.0),
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--gpu-memory-utilization",
				"0",
			},
		},
		{
			name: "with GPU memory utilization 1.0 (edge case)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(1.0),
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--gpu-memory-utilization",
				"1",
			},
		},
		{
			name: "with GPU memory utilization negative (invalid)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(-0.1),
				},
			},
			expectError: true,
		},
		{
			name: "with GPU memory utilization > 1.0 (invalid)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(1.5),
				},
			},
			expectError: true,
		},
		{
			name: "with GPU memory utilization and other parameters",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				ContextSize: int32ptr(8192),
				VLLM: &inference.VLLMConfig{
					GPUMemoryUtilization: float64ptr(0.7),
					HFOverrides: inference.HFOverrides{
						"architectures": []interface{}{"LlamaForCausalLM"},
					},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--max-model-len",
				"8192",
				"--gpu-memory-utilization",
				"0.7",
				"--hf-overrides",
				`{"architectures":["LlamaForCausalLM"]}`,
			},
		},
		{
			name: "without GPU memory utilization (should not add flag)",
			bundle: &mockModelBundle{
				safetensorsPath: "/path/to/model",
			},
			config: &inference.BackendConfiguration{
				VLLM: &inference.VLLMConfig{
					HFOverrides: inference.HFOverrides{
						"model_type": "llama",
					},
				},
			},
			expected: []string{
				"serve",
				"/path/to",
				"--uds",
				"/tmp/socket",
				"--hf-overrides",
				`{"model_type":"llama"}`,
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

func int32ptr(n int32) *int32 {
	return &n
}

func float64ptr(n float64) *float64 {
	return &n
}
