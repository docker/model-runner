package mlx

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the MLX backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultMLXConfig creates a new MLXConfig with default values.
func NewDefaultMLXConfig() *Config {
	return &Config{
		Args: []string{},
	}
}

// GetArgs implements BackendConfig.GetArgs.
func (c *Config) GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Start with the arguments from MLXConfig
	args := append([]string{}, c.Args...)

	// MLX uses Python module: python -m mlx_lm.server
	args = append(args, "-m", "mlx_lm.server")

	// Add model path (MLX works with safetensors format)
	safetensorsPath := bundle.SafetensorsPath()
	if safetensorsPath == "" {
		return nil, fmt.Errorf("safetensors path required by MLX backend")
	}
	modelPath := filepath.Dir(safetensorsPath)

	// Add model and socket arguments
	args = append(args, "--model", modelPath, "--host", socket)

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Default mode for MLX
	case inference.BackendModeEmbedding:
		// MLX doesn't have a specific embedding flag - embedding models are detected automatically
	case inference.BackendModeReranking:
		// MLX may not support reranking mode
		return nil, fmt.Errorf("reranking mode not supported by MLX backend")
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add max-tokens if specified in model config or backend config
	if maxLen := GetMaxTokens(bundle.RuntimeConfig(), config); maxLen != nil {
		args = append(args, "--max-tokens", strconv.FormatUint(*maxLen, 10))
	}

	return args, nil
}

// GetMaxTokens returns the max tokens (context size) from model config or backend config.
// Model config takes precedence over backend config.
// Returns nil if neither is specified (MLX will use model defaults).
func GetMaxTokens(modelCfg types.Config, backendCfg *inference.BackendConfiguration) *uint64 {
	return nil
}
