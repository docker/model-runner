package models

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/types"
)

// Backend represents the inference backend to use for a model.
type Backend string

const (
	// BackendLlamaServer uses llama.cpp's llama-server for GGUF models.
	BackendLlamaServer Backend = "llama-server"
	// BackendVLLM uses vLLM for safetensors models.
	BackendVLLM Backend = "vllm"
)

// DetectBackend determines the appropriate inference backend based on the model format.
func DetectBackend(bundle types.ModelBundle) Backend {
	// Check model format from runtime config if available
	if cfg := bundle.RuntimeConfig(); cfg != nil {
		switch cfg.GetFormat() {
		case types.FormatGGUF:
			return BackendLlamaServer
		case types.FormatSafetensors:
			return BackendVLLM
		case types.FormatDiffusers:
			return BackendVLLM
		default:
			// Default to llama-server for unknown formats
			return BackendLlamaServer
		}
	}

	// Fall back to checking paths
	if bundle.GGUFPath() != "" {
		return BackendLlamaServer
	}
	if bundle.SafetensorsPath() != "" {
		return BackendVLLM
	}

	// Default to llama-server
	return BackendLlamaServer
}

// BackendImage returns the container image to use for the given backend and GPU type.
func BackendImage(backend Backend, gpuType string) string {
	switch backend {
	case BackendVLLM:
		switch gpuType {
		case "nvidia":
			return "docker.io/docker/model-runner:latest-vllm-cuda"
		case "amd":
			return "docker.io/docker/model-runner:latest-vllm-rocm"
		default:
			// vLLM requires GPU
			return "docker.io/docker/model-runner:latest-vllm-cuda"
		}
	case BackendLlamaServer:
		switch gpuType {
		case "nvidia":
			return "docker.io/docker/model-runner:cuda"
		case "amd":
			return "docker.io/docker/model-runner:rocm"
		default:
			return "docker.io/docker/model-runner:latest"
		}
	default:
		return "docker.io/docker/model-runner:latest"
	}
}

// BackendCommand returns the command to run for the given backend.
func BackendCommand(backend Backend, modelPath string, port int) []string {
	switch backend {
	case BackendVLLM:
		return []string{
			"vllm", "serve", modelPath,
			"--port", fmt.Sprintf("%d", port),
		}
	case BackendLlamaServer:
		return []string{
			"llama-server",
			"--model", modelPath,
			"--port", fmt.Sprintf("%d", port),
			"--host", "0.0.0.0",
		}
	default:
		return []string{
			"llama-server",
			"--model", modelPath,
			"--port", fmt.Sprintf("%d", port),
			"--host", "0.0.0.0",
		}
	}
}
