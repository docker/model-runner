package llamacpp

import (
	"fmt"
	"runtime"
	"strconv"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the llama.cpp backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultLlamaCppConfig creates a new LlamaCppConfig with default values.
func NewDefaultLlamaCppConfig() *Config {
	args := append([]string{"-ngl", "999", "--metrics"})

	// Special case for ARM64
	if runtime.GOARCH == "arm64" {
		// Using a thread count equal to core count results in bad performance, and there seems to be little to no gain
		// in going beyond core_count/2.
		if !containsArg(args, "--threads") {
			nThreads := max(2, runtime.NumCPU()/2)
			args = append(args, "--threads", strconv.Itoa(nThreads))
		}
	}

	return &Config{
		Args: args,
	}
}

// GetArgs implements BackendConfig.GetArgs.
func (c *Config) GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Start with the arguments from LlamaCppConfig
	args := append([]string{}, c.Args...)

	modelPath := bundle.GGUFPath()
	if modelPath == "" {
		return nil, fmt.Errorf("GGUF file required by llama.cpp backend")
	}

	// Add model and socket arguments
	args = append(args, "--model", modelPath, "--host", socket)

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Add arguments for chat template file
		if path := bundle.ChatTemplatePath(); path != "" {
			args = append(args, "--chat-template-file", path)
		}
	case inference.BackendModeEmbedding:
		args = append(args, "--embeddings")
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	// Add context size from model config or backend config
	args = append(args, "--ctx-size", strconv.FormatUint(GetContextSize(bundle.RuntimeConfig(), config), 10))

	// Add arguments from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	// Add arguments for Multimodal projector or jinja (they are mutually exclusive)
	if path := bundle.MMPROJPath(); path != "" {
		args = append(args, "--mmproj", path)
	} else {
		args = append(args, "--jinja")
	}

	return args, nil
}

func GetContextSize(modelCfg types.Config, backendCfg *inference.BackendConfiguration) uint64 {
	// Model config takes precedence
	if modelCfg.ContextSize != nil {
		return *modelCfg.ContextSize
	}
	// else use backend config
	if backendCfg != nil && backendCfg.ContextSize > 0 {
		return uint64(backendCfg.ContextSize)
	}
	// finally return default
	return 4096 // llama.cpp default
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
