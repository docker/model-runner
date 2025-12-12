package sglang

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the SGLang backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultSGLangConfig creates a new SGLangConfig with default values.
func NewDefaultSGLangConfig() *Config {
	return &Config{}
}

// GetArgs implements BackendConfig.GetArgs.
func (c *Config) GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Start with the arguments from SGLangConfig
	args := append([]string{}, c.Args...)

	// SGLang uses Python module: python -m sglang.launch_server
	args = append(args, "-m", "sglang.launch_server")

	// Add model path
	safetensorsPath := bundle.SafetensorsPath()
	if safetensorsPath == "" {
		return nil, fmt.Errorf("safetensors path required by SGLang backend")
	}
	modelPath := filepath.Dir(safetensorsPath)
	args = append(args, "--model-path", modelPath)

	host, port, err := net.SplitHostPort(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host:port from %q: %w", socket, err)
	}
	args = append(args, "--host", host, "--port", port)

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Default mode for SGLang
	case inference.BackendModeEmbedding:
		args = append(args, "--is-embedding")
	case inference.BackendModeReranking:
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add context-length if specified in model config or backend config
	if contextLen := GetContextLength(bundle.RuntimeConfig(), config); contextLen != nil {
		args = append(args, "--context-length", strconv.FormatUint(*contextLen, 10))
	}

	// Add arguments from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	return args, nil
}

// GetContextLength returns the context length (context size) from model config or backend config.
// Model config takes precedence over backend config.
// Returns nil if neither is specified (SGLang will auto-derive from model).
func GetContextLength(modelCfg types.Config, backendCfg *inference.BackendConfiguration) *uint64 {
	// Model config takes precedence
	if modelCfg.ContextSize != nil {
		return modelCfg.ContextSize
	}
	// else use backend config
	if backendCfg != nil && backendCfg.ContextSize > 0 {
		val := uint64(backendCfg.ContextSize)
		return &val
	}
	// Return nil to let SGLang auto-derive from model config
	return nil
}
