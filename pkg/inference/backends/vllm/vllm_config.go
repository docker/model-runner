package vllm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the vLLM backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultVLLMConfig creates a new VLLMConfig with default values.
func NewDefaultVLLMConfig() *Config {
	return &Config{
		Args: []string{},
	}
}

// GetArgs implements BackendConfig.GetArgs.
func (c *Config) GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Start with the arguments from VLLMConfig
	args := append([]string{}, c.Args...)

	// Add the serve command and model path (use directory for safetensors)
	safetensorsPath := bundle.SafetensorsPath()
	if safetensorsPath == "" {
		return nil, fmt.Errorf("safetensors path required by vLLM backend")
	}
	modelPath := filepath.Dir(safetensorsPath)
	// vLLM expects the directory containing the safetensors files
	args = append(args, "serve", modelPath)

	// Add socket arguments
	args = append(args, "--uds", socket)

	// Add chat template if available in the model bundle.
	// Since transformers v4.44, vLLM no longer provides a default chat
	// template so we must supply one when the tokenizer omits it.
	if path := bundle.ChatTemplatePath(); path != "" {
		args = append(args, "--chat-template", path)
	}

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Default mode for vLLM
	case inference.BackendModeEmbedding:
		// Use pooling runner for embedding models
		args = append(args, "--runner", "pooling")
	case inference.BackendModeReranking:
		// vLLM does not have a specific flag for reranking
	case inference.BackendModeImageGeneration:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add max-model-len if specified in model config or backend config
	if maxLen := GetMaxModelLen(bundle.RuntimeConfig(), config); maxLen != nil {
		args = append(args, "--max-model-len", strconv.FormatInt(int64(*maxLen), 10))
	}

	// Add runtime flags from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	// Add vLLM-specific arguments from backend config
	if config != nil && config.VLLM != nil {
		// Add GPU memory utilization if specified
		if config.VLLM.GPUMemoryUtilization != nil {
			utilization := *config.VLLM.GPUMemoryUtilization
			if utilization < 0.0 || utilization > 1.0 {
				return nil, fmt.Errorf("gpu-memory-utilization must be between 0.0 and 1.0, got %f", utilization)
			}
			args = append(args, "--gpu-memory-utilization", strconv.FormatFloat(utilization, 'f', -1, 64))
		}

		// Add HuggingFace overrides if specified
		if len(config.VLLM.HFOverrides) > 0 {
			hfOverridesJSON, err := json.Marshal(config.VLLM.HFOverrides)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize hf-overrides: %w", err)
			}
			args = append(args, "--hf-overrides", string(hfOverridesJSON))

			// Extract known server parameters from hf_overrides.
			// vLLM ignores server parameters like tensor_parallel_size and max_model_len
			// when passed via --hf-overrides, so we extract them and pass them as
			// proper --tensor-parallel-size and --max-model-len flags.
			// See https://github.com/docker/model-runner/issues/912
			if ml, ok := config.VLLM.HFOverrides["max_model_len"]; ok {
				if m, ok := toInt(ml); ok {
					// Only add if not already added by GetMaxModelLen or RuntimeFlags
					if maxLen := GetMaxModelLen(bundle.RuntimeConfig(), config); maxLen == nil && !hasRuntimeFlag(config.RuntimeFlags, "--max-model-len") {
						args = append(args, "--max-model-len", strconv.Itoa(m))
					}
				}
			}
			if tpSize, ok := config.VLLM.HFOverrides["tensor_parallel_size"]; ok {
				if tp, ok := toInt(tpSize); ok && !hasRuntimeFlag(config.RuntimeFlags, "--tensor-parallel-size") {
					args = append(args, "--tensor-parallel-size", strconv.Itoa(tp))
				}
			}
		}
	}

	return args, nil
}

// toInt converts a numeric value from HFOverrides to an int.
// JSON numbers decode as float64, but programmatic construction may use int types.
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	default:
		return 0, false
	}
}

// hasRuntimeFlag checks whether the given flag key is already present in the
// runtime flags list. It handles both "--flag value" and "--flag=value" forms.
func hasRuntimeFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
		if len(f) > len(flag) && f[:len(flag)] == flag && f[len(flag)] == '=' {
			return true
		}
	}
	return false
}

// GetMaxModelLen returns the max model length (context size) from backend config or model config.
// Backend config takes precedence over model config (runtime configuration).
// Returns nil if neither is specified (vLLM will auto-derive from model).
func GetMaxModelLen(modelCfg types.ModelConfig, backendCfg *inference.BackendConfiguration) *int32 {
	// Backend config takes precedence (runtime configuration via docker model configure / Ollama API num_ctx)
	if backendCfg != nil && backendCfg.ContextSize != nil && *backendCfg.ContextSize > 0 {
		return backendCfg.ContextSize
	}
	// Fallback to model config (set at packaging time via docker model package --context-size)
	if modelCfg != nil {
		if ctxSize := modelCfg.GetContextSize(); ctxSize != nil && *ctxSize > 0 {
			return ctxSize
		}
	}
	// Return nil to let vLLM auto-derive from model config
	return nil
}
