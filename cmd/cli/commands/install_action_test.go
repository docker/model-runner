package commands

import (
	"testing"

	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
)

func TestResolveInstallAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		backend           string
		engineKind        types.ModelRunnerEngineKind
		supportsVLLMMetal bool
		isWSL             bool
		expected          installAction
	}{
		// === vllm backend ===
		{
			name:              "vllm + Desktop (darwin/arm64) → deferred vllm-metal",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindDesktop,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionDeferredVLLMMetal,
		},
		{
			name:              "vllm + Desktop+WSL (darwin/arm64) → create container",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindDesktop,
			supportsVLLMMetal: true,
			isWSL:             true,
			expected:          installActionCreateContainer,
		},
		{
			name:              "vllm + MobyManual (darwin/arm64) → deferred vllm-metal",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindMobyManual,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionDeferredVLLMMetal,
		},
		{
			name:              "vllm + Cloud → create container",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindCloud,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},
		{
			name:              "vllm + Moby → create container",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindMoby,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},
		{
			name:              "vllm + Moby (darwin/arm64, e.g. Colima) → create container",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindMoby,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},
		{
			name:              "vllm + Cloud (darwin/arm64) → create container",
			backend:           vllm.Name,
			engineKind:        types.ModelRunnerEngineKindCloud,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},

		// === diffusers backend ===
		{
			name:              "diffusers + Desktop → deferred diffusers",
			backend:           diffusers.Name,
			engineKind:        types.ModelRunnerEngineKindDesktop,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionDeferredDiffusers,
		},
		{
			name:              "diffusers + Moby → deferred diffusers",
			backend:           diffusers.Name,
			engineKind:        types.ModelRunnerEngineKindMoby,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionDeferredDiffusers,
		},
		{
			name:              "diffusers + Cloud → deferred diffusers",
			backend:           diffusers.Name,
			engineKind:        types.ModelRunnerEngineKindCloud,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionDeferredDiffusers,
		},

		// === llamacpp backend ===
		{
			name:              "llamacpp + Desktop → already in Desktop",
			backend:           llamacpp.Name,
			engineKind:        types.ModelRunnerEngineKindDesktop,
			supportsVLLMMetal: true,
			isWSL:             false,
			expected:          installActionAlreadyInDesktop,
		},
		{
			name:              "llamacpp + Moby → create container",
			backend:           llamacpp.Name,
			engineKind:        types.ModelRunnerEngineKindMoby,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},
		{
			name:              "llamacpp + Cloud → create container",
			backend:           llamacpp.Name,
			engineKind:        types.ModelRunnerEngineKindCloud,
			supportsVLLMMetal: false,
			isWSL:             false,
			expected:          installActionCreateContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := resolveInstallAction(
				tt.backend,
				tt.engineKind,
				tt.supportsVLLMMetal,
				tt.isWSL,
			)

			if result != tt.expected {
				t.Errorf("resolveInstallAction(%q, %v, supportsVLLMMetal=%v, isWSL=%v) = %d, want %d",
					tt.backend, tt.engineKind, tt.supportsVLLMMetal, tt.isWSL, result, tt.expected)
			}
		})
	}
}
