package diffusers

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the diffusers backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultConfig creates a new Config with default values.
func NewDefaultConfig() *Config {
	return &Config{
		Args: []string{},
	}
}

// GetArgs implements config.BackendConfig.GetArgs.
func (c *Config) GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	if mode != inference.BackendModeImageGeneration {
		return nil, fmt.Errorf("diffusers backend only supports image generation mode")
	}

	// Start with base arguments
	args := append([]string{}, c.Args...)

	// Python module entry point
	args = append(args, "-m", "diffusers_server")

	// Get model path
	modelPath, err := getModelPath(bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to get model path: %w", err)
	}

	// Add model path argument
	args = append(args, "--model-path", modelPath)

	// Add socket argument
	args = append(args, "--socket", socket)

	// Add runtime flags from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	// Add diffusers-specific arguments from backend config
	if config != nil && config.Diffusers != nil {
		if config.Diffusers.Device != "" {
			args = append(args, "--device", config.Diffusers.Device)
		}
		if config.Diffusers.Precision != "" {
			args = append(args, "--precision", config.Diffusers.Precision)
		}
		if config.Diffusers.EnableAttentionSlicing {
			args = append(args, "--enable-attention-slicing")
		}
	}

	return args, nil
}

// getModelPath extracts the model path from the bundle.
func getModelPath(bundle types.ModelBundle) (string, error) {
	rootDir := bundle.RootDir()
	if rootDir != "" {
		return rootDir, nil
	}

	return "", fmt.Errorf("no model path found in bundle")
}
