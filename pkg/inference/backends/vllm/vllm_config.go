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

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Default mode for vLLM
	case inference.BackendModeEmbedding:
	// vLLM doesn't have a specific embedding flag like llama.cpp
	// Embedding models are detected automatically
	case inference.BackendModeReranking:
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add max-model-len if specified in model config or backend config
	if maxLen := GetMaxModelLen(bundle.RuntimeConfig(), config); maxLen != nil {
		args = append(args, "--max-model-len", strconv.FormatInt(int64(*maxLen), 10))
	}
	// If nil, vLLM will automatically derive from the model config

	// Add vLLM-specific arguments from backend config
	if config != nil && config.VLLM != nil {
		// Add HuggingFace overrides if specified
		if len(config.VLLM.HFOverrides) > 0 {
			hfOverridesJSON, err := json.Marshal(config.VLLM.HFOverrides)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize hf-overrides: %w", err)
			}
			args = append(args, "--hf-overrides", string(hfOverridesJSON))
		}
	}

	return args, nil
}

// GetMaxModelLen returns the max model length (context size) from model config or backend config.
// Model config takes precedence over backend config.
// Returns nil if neither is specified (vLLM will auto-derive from model).
func GetMaxModelLen(modelCfg types.Config, backendCfg *inference.BackendConfiguration) *int32 {
	// Model config takes precedence
	if modelCfg.ContextSize != nil {
		return modelCfg.ContextSize
	}
	// Fallback to backend config
	if backendCfg != nil && backendCfg.ContextSize != nil && *backendCfg.ContextSize > 0 {
		return backendCfg.ContextSize
	}
	// Return nil to let vLLM auto-derive from model config
	return nil
}
