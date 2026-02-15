package inference

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/dmrlet/models"
	"github.com/docker/model-runner/pkg/dmrlet/runtime"
)

// ContainerModelPath is the path where models are mounted inside containers.
const ContainerModelPath = "/model"

// SpecBuilder builds container specifications for inference workloads.
type SpecBuilder struct {
	gpu *runtime.GPUInfo
}

// NewSpecBuilder creates a new spec builder.
func NewSpecBuilder(gpu *runtime.GPUInfo) *SpecBuilder {
	return &SpecBuilder{gpu: gpu}
}

// Build creates a container spec for running inference on a model.
func (b *SpecBuilder) Build(modelRef string, bundle types.ModelBundle, backend models.Backend, port int) runtime.ContainerSpec {
	// Determine the model file path inside the container
	modelPath := b.getModelPath(bundle, backend)

	// Build environment variables
	env := b.buildEnv()

	// Build command
	cmd := models.BackendCommand(backend, modelPath, port)

	// Get the appropriate image
	gpuType := "none"
	if b.gpu != nil {
		gpuType = b.gpu.Type
	}
	image := models.BackendImage(backend, gpuType)

	// Create container ID from model reference
	containerID := sanitizeContainerID(modelRef)

	return runtime.ContainerSpec{
		ID:      containerID,
		Image:   image,
		Command: cmd,
		Env:     env,
		Mounts: []runtime.Mount{
			{
				Source:      bundle.RootDir(),
				Destination: ContainerModelPath,
				ReadOnly:    true,
			},
		},
		GPU:     b.gpu,
		HostNet: true,
	}
}

// getModelPath returns the path to the model file inside the container.
func (b *SpecBuilder) getModelPath(bundle types.ModelBundle, backend models.Backend) string {
	switch backend {
	case models.BackendVLLM:
		// vLLM needs the safetensors directory
		if safePath := bundle.SafetensorsPath(); safePath != "" {
			// Return the directory containing safetensors
			relPath, _ := filepath.Rel(bundle.RootDir(), filepath.Dir(safePath))
			return filepath.Join(ContainerModelPath, relPath)
		}
		return ContainerModelPath
	case models.BackendLlamaServer:
		// llama-server needs the GGUF file
		if ggufPath := bundle.GGUFPath(); ggufPath != "" {
			relPath, _ := filepath.Rel(bundle.RootDir(), ggufPath)
			return filepath.Join(ContainerModelPath, relPath)
		}
		return filepath.Join(ContainerModelPath, "model", "model.gguf")
	default:
		return ContainerModelPath
	}
}

// buildEnv builds environment variables for the container.
func (b *SpecBuilder) buildEnv() []string {
	var env []string

	// Add GPU-specific environment variables
	if b.gpu != nil {
		env = append(env, runtime.GPUEnvVars(b.gpu)...)
	}

	return env
}

// sanitizeContainerID creates a valid container ID from a model reference.
func sanitizeContainerID(ref string) string {
	// Replace invalid characters with hyphens
	id := strings.ReplaceAll(ref, "/", "-")
	id = strings.ReplaceAll(id, ":", "-")
	id = strings.ReplaceAll(id, ".", "-")

	// Ensure it starts with a letter
	if id != "" && (id[0] >= '0' && id[0] <= '9') {
		id = "m-" + id
	}

	// Truncate if too long
	if len(id) > 63 {
		id = id[:63]
	}

	return fmt.Sprintf("dmrlet-%s", id)
}

// ContainerIDFromRef returns the container ID that would be used for a model reference.
func ContainerIDFromRef(ref string) string {
	return sanitizeContainerID(ref)
}
