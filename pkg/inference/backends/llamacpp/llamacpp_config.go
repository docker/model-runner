package llamacpp

import (
	"fmt"
	"runtime"
	"strconv"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

const UnlimitedContextSize = -1

// Config is the configuration for the llama.cpp backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// NewDefaultLlamaCppConfig creates a new LlamaCppConfig with default values.
func NewDefaultLlamaCppConfig() *Config {
	args := []string{"-ngl", "999", "--metrics"}

	// Special case for macOS (Apple), optimization
	if runtime.GOOS == "darwin" {
		args = append(args, "--no-mmap")
	}

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
	case inference.BackendModeReranking:
		args = append(args, "--embeddings", "--reranking")
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}

	if budget := GetReasoningBudget(config); budget != nil {
		args = append(args, "--reasoning-budget", strconv.FormatInt(int64(*budget), 10))
	}

	// Add context size from model config or backend config
	contextSize := GetContextSize(bundle.RuntimeConfig(), config)
	if contextSize != nil {
		args = append(args, "--ctx-size", strconv.FormatInt(int64(*contextSize), 10))
	}

	// Add arguments for Multimodal projector or jinja (they are mutually exclusive)
	if path := bundle.MMPROJPath(); path != "" {
		args = append(args, "--mmproj", path)
	} else {
		args = append(args, "--jinja")
	}

	return args, nil
}

func GetContextSize(modelCfg types.Config, backendCfg *inference.BackendConfiguration) *int32 {
	// Model config takes precedence
	if modelCfg.ContextSize != nil && (*modelCfg.ContextSize == UnlimitedContextSize || *modelCfg.ContextSize > 0) {
		return modelCfg.ContextSize
	}
	// Fallback to backend config
	if backendCfg != nil && backendCfg.ContextSize != nil && (*backendCfg.ContextSize == UnlimitedContextSize || *backendCfg.ContextSize > 0) {
		return backendCfg.ContextSize
	}
	return nil
}

func GetReasoningBudget(backendCfg *inference.BackendConfiguration) *int32 {
	if backendCfg != nil && backendCfg.LlamaCpp != nil && backendCfg.LlamaCpp.ReasoningBudget != nil {
		return backendCfg.LlamaCpp.ReasoningBudget
	}
	return nil
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
