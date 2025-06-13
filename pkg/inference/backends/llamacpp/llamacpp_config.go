package llamacpp

import (
	"runtime"
	"strconv"

	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the llama.cpp backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultLlamaCppConfig creates a new LlamaCppConfig with default values.
func NewDefaultLlamaCppConfig() *Config {
	args := append([]string{"--jinja", "-ngl", "100", "--metrics"})

	// Special case for Windows ARM64
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		// Using a thread count equal to core count results in bad performance, and there seems to be little to no gain
		// in going beyond core_count/2.
		if !containsArg(args, "--threads") {
			nThreads := min(2, runtime.NumCPU()/2)
			args = append(args, "--threads", strconv.Itoa(nThreads))
		}
	}

	return &Config{
		Args: args,
	}
}

// GetArgs implements BackendConfig.GetArgs.
func (c *Config) GetArgs(modelPath, socket string, mode inference.BackendMode) []string {
	// Start with the arguments from LlamaCppConfig
	args := append([]string{}, c.Args...)

	// Add model and socket arguments
	args = append(args, "--model", modelPath, "--host", socket)

	// Add mode-specific arguments
	if mode == inference.BackendModeEmbedding {
		args = append(args, "--embeddings")
	}

	return args
}

// containsArg checks if the given argument is already in the args slice.
func containsArg(args []string, arg string) bool {
	for _, a := range args {
		if a == arg {
			return true
		}
	}
	return false
}
