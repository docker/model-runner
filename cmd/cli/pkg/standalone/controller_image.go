package standalone

import (
	"os"

	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
)

const (
	// ControllerImage is the image used for the controller container.
	ControllerImage = "docker/model-runner"
	// defaultControllerImageVersion is the image version used for the controller container
	defaultControllerImageVersion = "latest"
	// OllamaImage is the image used for the ollama controller container.
	OllamaImage = "ollama/ollama"
	// defaultOllamaImageVersion is the image version used for the ollama controller container
	defaultOllamaImageVersion = "latest"
)

func controllerImageVersion() string {
	if version, ok := os.LookupEnv("MODEL_RUNNER_CONTROLLER_VERSION"); ok && version != "" {
		return version
	}
	return defaultControllerImageVersion
}

func controllerImageVariant(detectedGPU gpupkg.GPUSupport) string {
	if variant, ok := os.LookupEnv("MODEL_RUNNER_CONTROLLER_VARIANT"); ok {
		if variant == "cpu" || variant == "generic" {
			return ""
		}
		return variant
	}
	switch detectedGPU {
	case gpupkg.GPUSupportCUDA:
		return "cuda"
	default:
		return ""
	}
}

func fmtControllerImageName(repo, version, variant string) string {
	tag := repo + ":" + version
	if len(variant) > 0 {
		tag += "-" + variant
	}
	return tag
}

func controllerImageName(detectedGPU gpupkg.GPUSupport) string {
	return fmtControllerImageName(ControllerImage, controllerImageVersion(), controllerImageVariant(detectedGPU))
}

func ollamaImageVersion() string {
	if version, ok := os.LookupEnv("OLLAMA_CONTROLLER_VERSION"); ok && version != "" {
		return version
	}
	return defaultOllamaImageVersion
}

func ollamaImageVariant(detectedGPU gpupkg.GPUSupport) string {
	if variant, ok := os.LookupEnv("OLLAMA_CONTROLLER_VARIANT"); ok {
		return variant
	}
	// For ollama, we have "rocm" variant for AMD GPUs
	// Note: CUDA GPUs use the base "latest" image
	if detectedGPU == gpupkg.GPUSupportCUDA {
		return "" // ollama/ollama:latest works for CUDA
	}
	return ""
}

func ollamaImageName(detectedGPU gpupkg.GPUSupport, gpuVariant string) string {
	variant := ollamaImageVariant(detectedGPU)
	// Allow explicit override with gpuVariant parameter (e.g., "rocm")
	if gpuVariant == "rocm" {
		variant = "rocm"
	}
	return fmtControllerImageName(OllamaImage, ollamaImageVersion(), variant)
}
