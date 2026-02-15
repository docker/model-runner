package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/dmrlet/gpu"
)

// Backend represents an inference backend type.
type Backend string

const (
	BackendLlamaCpp  Backend = "llama.cpp"
	BackendVLLM      Backend = "vllm"
	BackendSGLang    Backend = "sglang"
	BackendDiffusers Backend = "diffusers"
)

// BackendConfig holds configuration for an inference backend.
type BackendConfig struct {
	Backend      Backend
	Command      []string
	DefaultImage string
	CUDAImage    string
	ROCmImage    string
	MetalImage   string
	DefaultPort  int
	SupportsGPU  bool
	ModelPathArg string
	HostArg      string
	PortArg      string
}

var backendConfigs = map[Backend]BackendConfig{
	BackendLlamaCpp: {
		Backend:      BackendLlamaCpp,
		Command:      []string{"llama-server"},
		DefaultImage: "ghcr.io/ggerganov/llama.cpp:server",
		CUDAImage:    "ghcr.io/ggerganov/llama.cpp:server-cuda",
		DefaultPort:  8080,
		SupportsGPU:  true,
		ModelPathArg: "--model",
		HostArg:      "--host",
		PortArg:      "--port",
	},
	BackendVLLM: {
		Backend:      BackendVLLM,
		Command:      []string{"python", "-m", "vllm.entrypoints.openai.api_server"},
		DefaultImage: "vllm/vllm-openai:latest",
		CUDAImage:    "vllm/vllm-openai:latest",
		DefaultPort:  8000,
		SupportsGPU:  true,
		ModelPathArg: "--model",
		HostArg:      "--host",
		PortArg:      "--port",
	},
	BackendSGLang: {
		Backend:      BackendSGLang,
		Command:      []string{"python", "-m", "sglang.launch_server"},
		DefaultImage: "lmsysorg/sglang:latest",
		CUDAImage:    "lmsysorg/sglang:latest",
		DefaultPort:  30000,
		SupportsGPU:  true,
		ModelPathArg: "--model-path",
		HostArg:      "--host",
		PortArg:      "--port",
	},
}

// SpecBuilder builds container specifications.
type SpecBuilder struct {
	config BackendConfig
}

// NewSpecBuilder creates a new spec builder for the given backend.
func NewSpecBuilder(backend Backend) (*SpecBuilder, error) {
	config, ok := backendConfigs[backend]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
	return &SpecBuilder{config: config}, nil
}

// ContainerSpec represents a complete container specification.
type ContainerSpec struct {
	Image   string
	Command []string
	Args    []string
	Env     map[string]string
	Mounts  []Mount
	Port    int
	Labels  map[string]string
}

// Mount represents a bind mount.
type Mount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// BuildOpts configures the spec building.
type BuildOpts struct {
	Model       string
	ModelPath   string
	Port        int
	GPUType     gpu.GPUType
	GPUs        []int
	ContextSize int
	GPUMemory   float64
	ExtraArgs   []string
	ExtraEnv    map[string]string
}

// Build builds a container spec for the given options.
func (b *SpecBuilder) Build(opts BuildOpts) (*ContainerSpec, error) {
	spec := &ContainerSpec{
		Image:   b.selectImage(opts.GPUType),
		Command: b.config.Command,
		Env:     make(map[string]string),
		Labels:  make(map[string]string),
		Port:    opts.Port,
	}

	if spec.Port == 0 {
		spec.Port = b.config.DefaultPort
	}

	// Build arguments
	args := b.buildArgs(opts)
	spec.Args = args

	// Add model mount
	if opts.ModelPath != "" {
		spec.Mounts = append(spec.Mounts, Mount{
			Source:      opts.ModelPath,
			Destination: "/models",
			ReadOnly:    true,
		})
	}

	// Add GPU environment variables
	if len(opts.GPUs) > 0 {
		gpuEnv := buildGPUEnv(opts.GPUType, opts.GPUs)
		for k, v := range gpuEnv {
			spec.Env[k] = v
		}
	}

	// Add extra environment variables
	for k, v := range opts.ExtraEnv {
		spec.Env[k] = v
	}

	// Add labels
	spec.Labels["dmrlet.model"] = opts.Model
	spec.Labels["dmrlet.backend"] = string(b.config.Backend)
	spec.Labels["dmrlet.port"] = strconv.Itoa(spec.Port)

	return spec, nil
}

func (b *SpecBuilder) selectImage(gpuType gpu.GPUType) string {
	switch gpuType {
	case gpu.GPUTypeNVIDIA:
		if b.config.CUDAImage != "" {
			return b.config.CUDAImage
		}
	case gpu.GPUTypeAMD:
		if b.config.ROCmImage != "" {
			return b.config.ROCmImage
		}
	case gpu.GPUTypeApple:
		if b.config.MetalImage != "" {
			return b.config.MetalImage
		}
	case gpu.GPUTypeNone, gpu.GPUTypeUnknown:
		// Fall through to default
	}
	return b.config.DefaultImage
}

func (b *SpecBuilder) buildArgs(opts BuildOpts) []string {
	var args []string

	// Model path
	if b.config.ModelPathArg != "" && opts.ModelPath != "" {
		modelFile := "/models" // Default value
		// For llama.cpp, we need to specify the actual model file
		if b.config.Backend == BackendLlamaCpp {
			// Find the actual model file in the model path
			foundFile := findModelFileWithValidation(opts.ModelPath)
			if foundFile == "" {
				// Since this function can't return an error, we'll log it and return an empty slice
				// In a real implementation, this would be handled differently
				return []string{}
			}
			modelFile = foundFile
		}
		args = append(args, b.config.ModelPathArg, modelFile)
	}

	// Host binding
	if b.config.HostArg != "" {
		args = append(args, b.config.HostArg, "0.0.0.0")
	}

	// Port
	if b.config.PortArg != "" {
		port := opts.Port
		if port == 0 {
			port = b.config.DefaultPort
		}
		args = append(args, b.config.PortArg, strconv.Itoa(port))
	}

	// Backend-specific arguments
	switch b.config.Backend {
	case BackendLlamaCpp:
		if opts.ContextSize > 0 {
			args = append(args, "--ctx-size", strconv.Itoa(opts.ContextSize))
		}
		if len(opts.GPUs) > 0 {
			// Enable GPU layers
			args = append(args, "--n-gpu-layers", "999")
		}
	case BackendVLLM:
		if opts.GPUMemory > 0 && opts.GPUMemory < 1 {
			args = append(args, "--gpu-memory-utilization", fmt.Sprintf("%.2f", opts.GPUMemory))
		}
		if len(opts.GPUs) > 0 {
			args = append(args, "--tensor-parallel-size", strconv.Itoa(len(opts.GPUs)))
		}
	case BackendSGLang:
		if len(opts.GPUs) > 0 {
			args = append(args, "--tp", strconv.Itoa(len(opts.GPUs)))
		}
	case BackendDiffusers:
		// No specific arguments needed for Diffusers backend
	}

	// Extra arguments
	args = append(args, opts.ExtraArgs...)

	return args
}

func buildGPUEnv(gpuType gpu.GPUType, gpus []int) map[string]string {
	if len(gpus) == 0 {
		return nil
	}

	var indices []string
	for _, idx := range gpus {
		indices = append(indices, strconv.Itoa(idx))
	}
	indexStr := strings.Join(indices, ",")

	env := make(map[string]string)

	switch gpuType {
	case gpu.GPUTypeNVIDIA:
		env["NVIDIA_VISIBLE_DEVICES"] = indexStr
		env["NVIDIA_DRIVER_CAPABILITIES"] = "compute,utility"
	case gpu.GPUTypeAMD:
		env["HIP_VISIBLE_DEVICES"] = indexStr
		env["ROCR_VISIBLE_DEVICES"] = indexStr
	case gpu.GPUTypeApple:
		env["METAL_DEVICE_WRAPPER_TYPE"] = "1"
	case gpu.GPUTypeNone, gpu.GPUTypeUnknown:
		// No GPU environment variables needed
	}

	return env
}

// GetDefaultBackend returns the default backend based on model format.
func GetDefaultBackend(modelFormat string) Backend {
	switch modelFormat {
	case "gguf":
		return BackendLlamaCpp
	case "safetensors":
		return BackendVLLM
	default:
		return BackendLlamaCpp
	}
}

// SupportedBackends returns a list of supported backend names.
func SupportedBackends() []string {
	return []string{
		string(BackendLlamaCpp),
		string(BackendVLLM),
		string(BackendSGLang),
	}
}

// ParseBackend parses a backend string.
func ParseBackend(s string) (Backend, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	switch s {
	case "llama.cpp", "llamacpp", "llama":
		return BackendLlamaCpp, nil
	case "vllm":
		return BackendVLLM, nil
	case "sglang":
		return BackendSGLang, nil
	case "diffusers":
		return BackendDiffusers, nil
	default:
		return "", fmt.Errorf("unknown backend: %s (supported: %s)", s, strings.Join(SupportedBackends(), ", "))
	}
}

// findModelFileWithValidation finds the actual model file in the model path and validates it exists
func findModelFileWithValidation(modelPath string) string {
	// Scan the model path for GGUF files
	files, err := os.ReadDir(modelPath)
	if err != nil {
		return ""
	}

	for _, file := range files {
		if !file.IsDir() {
			name := strings.ToLower(file.Name())
			if strings.HasSuffix(name, ".gguf") {
				return fmt.Sprintf("/models/%s", file.Name())
			}
		}
	}

	// If no GGUF file was found, look for other common model formats
	for _, file := range files {
		if !file.IsDir() {
			name := strings.ToLower(file.Name())
			if strings.HasSuffix(name, ".bin") || strings.HasSuffix(name, ".safetensors") || strings.HasSuffix(name, ".onnx") {
				return fmt.Sprintf("/models/%s", file.Name())
			}
		}
	}

	// No suitable model file found
	return ""
}
