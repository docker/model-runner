package scheduling

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/mlx"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"log/slog"
)

// mockPlatformSupport allows tests to control platform capability checks.
type mockPlatformSupport struct {
	mlx       bool
	vllm      bool
	vllmMetal bool
	sglang    bool
}

func (m mockPlatformSupport) SupportsMLX() bool       { return m.mlx }
func (m mockPlatformSupport) SupportsVLLM() bool      { return m.vllm }
func (m mockPlatformSupport) SupportsVLLMMetal() bool { return m.vllmMetal }
func (m mockPlatformSupport) SupportsSGLang() bool    { return m.sglang }

// mockModel is a minimal Model implementation for testing.
type mockModel struct {
	config types.ModelConfig
}

func (m *mockModel) ID() (string, error)                   { return "test-id", nil }
func (m *mockModel) GGUFPaths() ([]string, error)          { return nil, nil }
func (m *mockModel) SafetensorsPaths() ([]string, error)   { return nil, nil }
func (m *mockModel) DDUFPaths() ([]string, error)          { return nil, nil }
func (m *mockModel) ConfigArchivePath() (string, error)    { return "", nil }
func (m *mockModel) MMPROJPath() (string, error)           { return "", nil }
func (m *mockModel) Config() (types.ModelConfig, error)    { return m.config, nil }
func (m *mockModel) Tags() []string                        { return []string{"test:latest"} }
func (m *mockModel) Descriptor() (types.Descriptor, error) { return types.Descriptor{}, nil }
func (m *mockModel) ChatTemplatePath() (string, error)     { return "", nil }

func newTestSchedulerWithPlatform(backends map[string]inference.Backend, defaultBackend inference.Backend, ps PlatformSupport) *Scheduler {
	log := slog.Default()

	s := NewScheduler(log, backends, defaultBackend, nil, nil, nil, nil)
	s.platformSupport = ps
	return s
}

func TestSelectBackendForModel(t *testing.T) {
	t.Parallel()

	llamacppBackend := &mockBackend{name: "llamacpp"}
	mlxBackend := &mockBackend{name: mlx.Name}
	vllmBackend := &mockBackend{name: vllm.Name}
	sglangBackend := &mockBackend{name: sglang.Name}

	safetensorsModel := &mockModel{config: &types.Config{Format: types.FormatSafetensors}}
	ggufModel := &mockModel{config: &types.Config{Format: types.FormatGGUF}}

	tests := []struct {
		name            string
		backends        map[string]inference.Backend
		defaultBackend  inference.Backend
		platform        mockPlatformSupport
		model           types.Model
		expectedBackend string
	}{
		{
			name: "Linux with MLX and vLLM registered selects vLLM for safetensors",
			backends: map[string]inference.Backend{
				"llamacpp":  llamacppBackend,
				mlx.Name:    mlxBackend,
				vllm.Name:   vllmBackend,
				sglang.Name: sglangBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: false, vllm: true, sglang: true},
			model:           safetensorsModel,
			expectedBackend: vllm.Name,
		},
		{
			name: "macOS without vllm-metal support falls back to MLX for safetensors",
			backends: map[string]inference.Backend{
				"llamacpp":  llamacppBackend,
				mlx.Name:    mlxBackend,
				vllm.Name:   vllmBackend,
				sglang.Name: sglangBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: true, vllm: false, sglang: false},
			model:           safetensorsModel,
			expectedBackend: mlx.Name,
		},
		{
			name: "macOS ARM64 with vllm-metal support selects unified vllm for safetensors",
			backends: map[string]inference.Backend{
				"llamacpp":  llamacppBackend,
				mlx.Name:    mlxBackend,
				vllm.Name:   vllmBackend,
				sglang.Name: sglangBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: true, vllmMetal: true},
			model:           safetensorsModel,
			expectedBackend: vllm.Name,
		},
		{
			name: "Linux with only SGLang selects SGLang for safetensors",
			backends: map[string]inference.Backend{
				"llamacpp":  llamacppBackend,
				mlx.Name:    mlxBackend,
				sglang.Name: sglangBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: false, vllm: false, sglang: true},
			model:           safetensorsModel,
			expectedBackend: sglang.Name,
		},
		{
			name: "GGUF model returns default backend regardless of platform",
			backends: map[string]inference.Backend{
				"llamacpp": llamacppBackend,
				mlx.Name:   mlxBackend,
				vllm.Name:  vllmBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: false, vllm: true, sglang: true},
			model:           ggufModel,
			expectedBackend: "llamacpp",
		},
		{
			name: "no platform-compatible backend returns default for safetensors",
			backends: map[string]inference.Backend{
				"llamacpp": llamacppBackend,
				mlx.Name:   mlxBackend,
				vllm.Name:  vllmBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: false, vllm: false, sglang: false},
			model:           safetensorsModel,
			expectedBackend: "llamacpp",
		},
		{
			name: "Linux without MLX in backends still selects vLLM",
			backends: map[string]inference.Backend{
				"llamacpp": llamacppBackend,
				vllm.Name:  vllmBackend,
			},
			defaultBackend:  llamacppBackend,
			platform:        mockPlatformSupport{mlx: false, vllm: true, sglang: false},
			model:           safetensorsModel,
			expectedBackend: vllm.Name,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newTestSchedulerWithPlatform(tt.backends, tt.defaultBackend, tt.platform)
			result := s.selectBackendForModel(tt.model, tt.defaultBackend, "test-model")

			if result.Name() != tt.expectedBackend {
				t.Errorf("expected backend %q, got %q", tt.expectedBackend, result.Name())
			}
		})
	}
}
