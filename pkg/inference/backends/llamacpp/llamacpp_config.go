package llamacpp

import (
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/docker/model-distribution/types"

	"github.com/docker/model-runner/pkg/inference"
)

// Config is the configuration for the llama.cpp backend.
type Config struct {
	// Args are the base arguments that are always included.
	Args []string
}

// isTerminal returns true if the given file is a character device (a terminal).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}

	return (fi.Mode() & os.ModeCharDevice) != 0
}

func stdoutIsTerminal() bool { return isTerminal(os.Stdout) }
func stderrIsTerminal() bool { return isTerminal(os.Stderr) }

func shouldColorize() bool {
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	return stdoutIsTerminal() && stderrIsTerminal()
}

// NewDefaultLlamaCppConfig creates a new LlamaCppConfig with default values.
func NewDefaultLlamaCppConfig() *Config {
	args := append([]string{"--jinja", "-ngl", "100", "--metrics"})
	if shouldColorize() {
		args = append(args, "--log-colors")
	}

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
	if mode == inference.BackendModeEmbedding {
		args = append(args, "--embeddings")
	}

	// Add context size from model config or backend config
	args = append(args, "--ctx-size", strconv.FormatUint(GetContextSize(bundle.RuntimeConfig(), config), 10))

	// Add arguments from backend config
	if config != nil {
		args = append(args, config.RuntimeFlags...)
	}

	// Add arguments for Multimodal projector
	if path := bundle.MMPROJPath(); path != "" {
		args = append(args, "--mmproj", path)
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
