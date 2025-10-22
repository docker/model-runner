package vllm

import (
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
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add max-model-len if specified in model config or backend config
	if maxLen := GetMaxModelLen(bundle.RuntimeConfig(), config); maxLen != nil {
		args = append(args, "--max-model-len", strconv.FormatUint(*maxLen, 10))
	}
	// If nil, vLLM will automatically derive from the model config

	// Add arguments from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	return args, nil
}

// GetMaxModelLen returns the max model length (context size) from model config or backend config.
// Model config takes precedence over backend config.
// Returns nil if neither is specified (vLLM will auto-derive from model).
func GetMaxModelLen(modelCfg types.Config, backendCfg *inference.BackendConfiguration) *uint64 {
	// Model config takes precedence
	if modelCfg.ContextSize != nil {
		return modelCfg.ContextSize
	}
	// else use backend config
	if backendCfg != nil && backendCfg.ContextSize > 0 {
		val := uint64(backendCfg.ContextSize)
		return &val
	}
	// Return nil to let vLLM auto-derive from model config
	return nil
}
