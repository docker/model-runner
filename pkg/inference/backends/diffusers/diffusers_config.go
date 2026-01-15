package diffusers

import (
	"fmt"
	"net"

	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the diffusers backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultConfig creates a new Config with default values.
func NewDefaultConfig() *Config {
	return &Config{}
}

// GetArgs implements BackendConfig.GetArgs for the diffusers backend.
func (c *Config) GetArgs(model string, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Start with the arguments from Config
	args := append([]string{}, c.Args...)

	// Diffusers uses Python module: python -m diffusers_server.server
	args = append(args, "-m", "diffusers_server.server")

	// Add model path - for diffusers this can be a HuggingFace model ID or local path
	args = append(args, "--model-path", model)

	// Parse host:port from socket
	host, port, err := net.SplitHostPort(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host:port from %q: %w", socket, err)
	}
	args = append(args, "--host", host, "--port", port)

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeImageGeneration:
		// Default mode for diffusers - image generation
	case inference.BackendModeCompletion, inference.BackendModeEmbedding, inference.BackendModeReranking:
		return nil, fmt.Errorf("unsupported backend mode %q for diffusers", mode)
	}

	return args, nil
}
