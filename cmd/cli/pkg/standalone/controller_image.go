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
)

func controllerImageVersion() string {
	if version, ok := os.LookupEnv("MODEL_RUNNER_CONTROLLER_VERSION"); ok && version != "" {
		return version
	}
	return defaultControllerImageVersion
}

func controllerImageVariant(detectedGPU gpupkg.GPUSupport, backend string) string {
	if variant, ok := os.LookupEnv("MODEL_RUNNER_CONTROLLER_VARIANT"); ok {
		if variant == "cpu" || variant == "generic" {
			return ""
		}
		return variant
	}
	// If vLLM backend is requested, return vllm-cuda variant
	if backend == "vllm" {
		return "vllm-cuda"
	}
	// Default to llama.cpp backend behavior
	switch detectedGPU {
	case gpupkg.GPUSupportCUDA:
		return "cuda"
	case gpupkg.GPUSupportROCm:
		return "rocm"
	case gpupkg.GPUSupportMUSA:
		return "musa"
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

func controllerImageName(detectedGPU gpupkg.GPUSupport, backend string) string {
	return fmtControllerImageName(ControllerImage, controllerImageVersion(), controllerImageVariant(detectedGPU, backend))
}
