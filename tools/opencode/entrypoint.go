package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// sizes defines the default, size-based configurations. The size, or individual
// configuration parameters, can be overridden using command line flags.
var sizes = map[string]struct {
	// defaultModel is the default model reference.
	defaultModel string
	// defaultModelContextSize is the default model context size.
	defaultModelContextSize uint64
	// defaultModelName is the default model's user-facing name.
	defaultModelName string
	// defaultModelFlags are the default model's llama.cpp flags.
	defaultModelFlags string
	// smallModel is the small model reference. It may be omitted to use the
	// default model for all tasks.
	smallModel string
	// smallModelContextSize is the small model context size. It has no effect
	// if smallModel is unspecified.
	smallModelContextSize uint64
	// smallModelName is the small model's user-facing name. It has no effect if
	// smallModel is unspecified.
	smallModelName string
	// smallModelFlags are the small model's llama.cpp flags.
	smallModelFlags string
}{
	// Recommended for systems with 16 GB of VRAM.
	// TODO: Add "small".
	//
	// An Apple M1 chip with 16 GB of unified memory offers a recommended Metal
	// working set size (VRAM size) of 5-12 GiB by default, depending on the
	// tool you ask. It's unclear if we have a viable model to operate at this
	// size with reasonable performance and context length.

	// Recommended for systems with 32 GB of VRAM.
	"medium": {
		// Default model. Parameters recommend here:
		// https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF
		// See "Best Practices".
		//
		// On Metal, this takes 11236.79 MiB of VRAM for tensor layers plus
		// 6096.00 MiB of VRAM for the KV cache (for a context length of 65000
		// tokens), for a total of 17332.79 MiB of VRAM.
		//
		// An Apple M2 Max chip with 32 GB of unified memory offers a
		// recommended Metal working set size (VRAM size) of 22906.50 MB
		// (21845.34 MiB) by default.
		"hf.co/unsloth/qwen3-coder-30b-a3b-instruct-gguf:q2_k_xl",
		65000,
		"Qwen3-Coder",
		"--temp 0.7 --top-p 0.8 --top-k 20 --repeat_penalty 1.05",

		// Small model (use default).
		"",
		0,
		"",
		"",
	},

	// Recommended for systems with 64 GB of VRAM.
	"large": {
		// Default model. Parameters recommend here:
		// https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF
		// See "Best Practices".
		//
		// On Metal, this takes 17691.35 MiB of VRAM for tensor layers plus
		// 18768.00 MiB of VRAM for the KV cache (for a context length of 200000
		// tokens), for a total of 36459.35 MiB of VRAM.
		"hf.co/unsloth/qwen3-coder-30b-a3b-instruct-gguf:q4_k_m",
		200000,
		"Qwen3-Coder",
		"--temp 0.7 --top-p 0.8 --top-k 20 --repeat_penalty 1.05",

		// Small model (use default).
		"",
		0,
		"",
		"",
	},

	// Recommended for systems with 128 GB of VRAM (or more).
	"xl": {
		// Default model. Parameters recommend here:
		// https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF
		// See "Best Practices".
		//
		// On Metal, this takes 34317.00 MiB of VRAM for tensor layers plus
		// 24576.00 MiB of VRAM for the KV cache (for a context length of
		// 262144 tokens), for a total of 58893.0 MiB of VRAM.
		//
		// An Apple M4 Max chip with 128 GB of unified memory offers a
		// recommended Metal working set size (VRAM size) of 103079.22 MB
		// (98304.00 MiB) by default.
		"hf.co/unsloth/qwen3-coder-30b-a3b-instruct-gguf:q8_k_xl",
		262144, // Maximum for Qwen3-Coder.
		"Qwen3-Coder",
		"--temp 0.7 --top-p 0.8 --top-k 20 --repeat_penalty 1.05",

		// Small model (use default).
		// TODO: We can afford another model on these systems. Experiment to see
		// if we can get a performance speedup with (say) gemma3-qat.
		"",
		0,
		"",
		"",
	},
}

const (
	// opencodeAuthenticationConfigurationPath is the path to the opencode
	// authentication configuration file.
	opencodeAuthenticationConfigurationPath = "/root/.local/share/opencode/auth.json"
	// opencodeAuthenticationConfiguration is the opencode authentication
	// configuration for Docker Model Runner.
	opencodeAuthenticationConfiguration = `{"docker":{"type":"api","key":"docker"}}`
	// opencodeConfigurationPath is the path to the opencode configuration file.
	opencodeConfigurationPath = "/root/.config/opencode/opencode.json"
)

// opencodeProvider defines a single opencode provider.
type opencodeProvider struct {
	// NPM is the npm package to use for interacting with the provider's APIs.
	NPM string `json:"npm"`
	// Name is the provider's user-facing name.
	Name string `json:"name"`
	// Options defines options for the provider.
	Options map[string]string `json:"options"`
	// Models maps model references to their configuration parameters.
	Models map[string]map[string]any `json:"models"`
}

// opencodeConfiguration defines the subset of the opencode configuration file
// that we need to render.
type opencodeConfiguration struct {
	// Schema is the configuration schema URL.
	Schema string `json:"$schema"`
	// Provider maps provider names to their specifications.
	Provider map[string]*opencodeProvider `json:"provider"`
	// Model is the default model to use (in the form "provider/model").
	Model string `json:"model,omitempty"`
	// SmallModel is the default "small" model to use (in the form
	// "provider/model"). It is used for tasks like title generation. If this is
	// omitted, then Model will be used for all operations.
	SmallModel string `json:"small_model,omitempty"`
	// AutoUpdate sets the auto-update policy. If omitted from the configuration
	// file, it defaults to true (which we don't really want - containers should
	// remain fully baked), so we don't use omitempty here (thus making the
	// default false).
	AutoUpdate bool `json:"autoupdate"`
	// DisabledProviders lists providers to disable (even if defined and
	// authenticated).
	DisabledProviders []string `json:"disabled_providers,omitempty"`
	// TODO: Add "mcp" member and wire to Docker MCP toolkit:
	// https://opencode.ai/docs/mcp-servers
}

// pullModel pulls a model using the Docker Model CLI plugin. We use the CLI
// plugin so that we can take advantage of its progress reporting.
func pullModel(ctx context.Context, model string) error {
	fmt.Println("docker model pull", model)
	command := exec.CommandContext(ctx, "docker", "model", "pull", model)
	// HACK: Override the DMR URL so that we don't require /var/run/docker.sock.
	command.Env = append(os.Environ(), "MODEL_RUNNER_HOST=http://model-runner.docker.internal")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

// modelConfigurationRequest is used to specify a model configuration.
type modelConfigurationRequest struct {
	// Model is the model reference.
	Model string `json:"model"`
	// ContextSize is the target context size.
	ContextSize uint64 `json:"context-size,omitempty"`
	// RawRuntimeFlags are the llama.cpp runtime flags to use for the model.
	RawRuntimeFlags string `json:"raw-runtime-flags,omitempty"`
}

// configureModel sets the context size and runtime flags (if any) for a model.
func configureModel(ctx context.Context, model string, contextSize uint64, runtimeFlags string) error {
	// Log the operation.
	fmt.Println("Setting configuration for", model)
	if contextSize != 0 {
		fmt.Printf("  Context size: %d tokens\n", contextSize)
	}
	if runtimeFlags != "" {
		fmt.Printf("  Runtime flags: %q\n", runtimeFlags)
	}

	// Set up the request body.
	body, err := json.Marshal(&modelConfigurationRequest{
		Model:           model,
		ContextSize:     contextSize,
		RawRuntimeFlags: runtimeFlags,
	})
	if err != nil {
		return fmt.Errorf("unable to format request body: %w", err)
	}

	// Create the request.
	request, err := http.NewRequestWithContext(ctx,
		// TODO: Auto-detect environment to support Docker Offload.
		http.MethodPost, "http://model-runner.docker.internal/engines/_configure",
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		return fmt.Errorf("unable to construct request: %w", err)
	}

	// Perform the request and verify that it was successful.
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("unable to perform request: %w", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("received unexpected response status: %s", response.Status)
	}
	return nil
}

// run is the main entrypoint.
func run() error {
	// Parse command line flags.
	var size string
	var defaultModelOverride, defaultModelNameOverride, defaultModelFlagsOverride string
	var smallModelOverride, smallModelNameOverride, smallModelFlagsOverride string
	var defaultModelContextSizeOverride, smallModelContextSizeOverride uint64
	flag.StringVar(&size, "s", "medium", "Override default environment size")
	flag.StringVar(&defaultModelOverride, "M", "", "Override default model")
	flag.Uint64Var(&defaultModelContextSizeOverride, "C", 0, "Override default model context size")
	flag.StringVar(&defaultModelNameOverride, "N", "", "Override default model's user-facing name")
	flag.StringVar(&defaultModelFlagsOverride, "F", "", "Override default model's llama.cpp flags")
	flag.StringVar(&smallModelOverride, "m", "", "Override small model")
	flag.Uint64Var(&smallModelContextSizeOverride, "c", 0, "Override small model context size")
	flag.StringVar(&smallModelNameOverride, "n", "", "Override small model's user-facing name")
	flag.StringVar(&smallModelFlagsOverride, "f", "", "Override small model's llama.cpp flags")
	flag.Parse()

	// Determine parameters. If a model reference is changed, we'll invalidate
	// its existing configuration (thus reverting things to their default values
	// unless overridden) since it may no longer be valid.
	parameters, ok := sizes[size]
	if !ok {
		return fmt.Errorf("unknown size value: %q", size)
	}
	if defaultModelOverride != "" && defaultModelOverride != parameters.defaultModel {
		parameters.defaultModel = defaultModelOverride
		parameters.defaultModelContextSize = 0
		parameters.defaultModelName = "Default"
		parameters.defaultModelFlags = ""
	}
	if defaultModelContextSizeOverride != 0 {
		parameters.defaultModelContextSize = defaultModelContextSizeOverride
	}
	if defaultModelNameOverride != "" {
		parameters.defaultModelName = defaultModelNameOverride
	}
	if defaultModelFlagsOverride != "" {
		parameters.defaultModelFlags = defaultModelFlagsOverride
	}
	if smallModelOverride != "" && smallModelOverride != parameters.smallModel {
		parameters.smallModel = smallModelOverride
		parameters.smallModelContextSize = 0
		parameters.smallModelName = "Small"
		parameters.smallModelFlags = ""
	}
	if smallModelContextSizeOverride != 0 {
		parameters.smallModelContextSize = smallModelContextSizeOverride
	}
	if smallModelNameOverride != "" {
		parameters.smallModelName = smallModelNameOverride
	}
	if smallModelFlagsOverride != "" {
		parameters.smallModelFlags = smallModelFlagsOverride
	}

	// Verify that the code directory exists.
	if s, err := os.Stat("/code"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.New("/code directory does not exist (you need to bind mount something there with -v <path>:/code)")
		}
		return fmt.Errorf("unable to probe /code directory: %w", err)
	} else if !s.IsDir() {
		return errors.New("/code doesn't point to a directory (you need to bind mount a directory with -v <path>:/code)")
	}

	// Write the opencode authentication configuration.
	if err := os.WriteFile(
		opencodeAuthenticationConfigurationPath,
		[]byte(opencodeAuthenticationConfiguration),
		0600,
	); err != nil {
		return fmt.Errorf("unable to write opencode authentication configuration: %w", err)
	}

	// Write the opencode configuration.
	configuration := &opencodeConfiguration{
		Schema: "https://opencode.ai/config.json",
		Provider: map[string]*opencodeProvider{
			"docker": {
				NPM:  "@ai-sdk/openai-compatible",
				Name: "Docker",
				Options: map[string]string{
					// TODO: Auto-detect environment to support Docker Offload.
					"baseURL": "http://model-runner.docker.internal/engines/v1",
				},
				Models: map[string]map[string]any{
					parameters.defaultModel: {
						"name": parameters.defaultModelName,
						"limit": map[string]uint64{
							"context": parameters.defaultModelContextSize,
							"output":  0, // Unlimited.
						},
					},
				},
			},
		},
		Model:             parameters.defaultModel,
		AutoUpdate:        false,
		DisabledProviders: []string{"opencode"},
	}
	if parameters.smallModel != "" {
		configuration.SmallModel = parameters.smallModel
		configuration.Provider["docker"].Models[parameters.smallModel] = map[string]any{
			"name": parameters.smallModelName,
			"limit": map[string]uint64{
				"context": parameters.smallModelContextSize,
				"output":  0, // Unlimited.
			},
		}
	}
	configurationData, err := json.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("unable to generate opencode configuration: %w", err)
	}
	if err := os.WriteFile(
		opencodeConfigurationPath,
		[]byte(configurationData),
		0600,
	); err != nil {
		return fmt.Errorf("unable to write opencode configuration: %w", err)
	}

	// Set up signal capture and forwarding for subprocesses.
	cmdCtx, freeSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer freeSignals()

	// Pull any required models and set their target context lengths.
	if err := pullModel(cmdCtx, parameters.defaultModel); err != nil {
		return fmt.Errorf("unable to pull default model")
	} else if err = configureModel(cmdCtx,
		parameters.defaultModel,
		parameters.defaultModelContextSize,
		parameters.defaultModelFlags,
	); err != nil {
		return fmt.Errorf("unable to configure default model")
	}
	if parameters.smallModel != "" {
		if err := pullModel(cmdCtx, parameters.smallModel); err != nil {
			return fmt.Errorf("unable to pull small model")
		} else if err = configureModel(cmdCtx,
			parameters.smallModel,
			parameters.smallModelContextSize,
			parameters.smallModelFlags,
		); err != nil {
			return fmt.Errorf("unable to configure small model")
		}
	}

	// Run opencode.
	opencode := exec.CommandContext(cmdCtx, "opencode")
	opencode.Dir = "/code"
	opencode.Stdin = os.Stdin
	opencode.Stdout = os.Stdout
	opencode.Stderr = os.Stderr
	return opencode.Run()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}
}
