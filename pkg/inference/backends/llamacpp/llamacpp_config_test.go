package llamacpp

import (
	"runtime"
	"slices"
	"strconv"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"

	"github.com/docker/model-runner/pkg/inference"
)

func TestNewDefaultLlamaCppConfig(t *testing.T) {
	config := NewDefaultLlamaCppConfig()

	// Test that --jinja is NOT in default args (it will be added conditionally in GetArgs)
	if containsArg(config.Args, "--jinja") {
		t.Error("Did not expect --jinja argument in default config (it should be added conditionally)")
	}

	// Test -ngl argument and its value
	nglIndex := -1
	for i, arg := range config.Args {
		if arg == "-ngl" {
			nglIndex = i
			break
		}
	}
	if nglIndex == -1 {
		t.Error("Expected -ngl argument to be present")
	}
	if nglIndex+1 >= len(config.Args) {
		t.Error("No value found after -ngl argument")
	}
	if config.Args[nglIndex+1] != "999" {
		t.Errorf("Expected -ngl value to be 999, got %s", config.Args[nglIndex+1])
	}

	// Test macOS (Apple) specific case
	if runtime.GOOS == "darwin" {
		if !containsArg(config.Args, "--no-mmap") {
			t.Error("Expected --no-mmap argument to be present on macOS")
		}
	} else if containsArg(config.Args, "--no-mmap") {
		// On non-macOS systems, --no-mmap should not be present by default
		t.Error("Did not expect --no-mmap argument to be present on non-macOS systems")
	}

	// Test Windows ARM64 specific case
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		if !containsArg(config.Args, "--threads") {
			t.Error("Expected --threads argument to be present on Windows ARM64")
		}
		threadsIndex := -1
		for i, arg := range config.Args {
			if arg == "--threads" {
				threadsIndex = i
				break
			}
		}
		if threadsIndex == -1 {
			t.Error("Could not find --threads argument")
		}
		if threadsIndex+1 >= len(config.Args) {
			t.Error("No value found after --threads argument")
		}
		threads, err := strconv.Atoi(config.Args[threadsIndex+1])
		if err != nil {
			t.Errorf("Failed to parse thread count: %v", err)
		}
		if threads > runtime.NumCPU()/2 {
			t.Errorf("Thread count %d exceeds maximum allowed value of %d", threads, runtime.NumCPU()/2)
		}
		if threads < 1 {
			t.Error("Thread count is less than 1")
		}
	}
}

func TestGetArgs(t *testing.T) {
	config := NewDefaultLlamaCppConfig()
	modelPath := "/path/to/model"
	socket := "unix:///tmp/socket"

	// Build base expected args based on architecture and OS
	baseArgs := []string{"-ngl", "999", "--metrics"}
	if runtime.GOOS == "darwin" {
		baseArgs = append(baseArgs, "--no-mmap")
	}
	if runtime.GOARCH == "arm64" {
		nThreads := max(2, runtime.NumCPU()/2)
		baseArgs = append(baseArgs, "--threads", strconv.Itoa(nThreads))
	}

	tests := []struct {
		name     string
		bundle   types.ModelBundle
		mode     inference.BackendMode
		config   *inference.BackendConfiguration
		expected []string
	}{
		{
			name: "completion mode",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "embedding mode",
			mode: inference.BackendModeEmbedding,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "context size from backend config",
			mode: inference.BackendModeEmbedding,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				ContextSize: 1234,
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "1234",
				"--jinja",
			),
		},
		{
			name: "context size from model config",
			mode: inference.BackendModeEmbedding,
			bundle: &fakeBundle{
				ggufPath: modelPath,
				config: types.Config{
					ContextSize: uint64ptr(2096),
				},
			},
			config: &inference.BackendConfiguration{
				ContextSize: 1234,
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "2096", // model config takes precedence
				"--jinja",
			),
		},
		{
			name: "chat template from model artifact",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath:     modelPath,
				templatePath: "/path/to/bundle/template.jinja",
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--chat-template-file", "/path/to/bundle/template.jinja",
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "multimodal projector removes jinja",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath:   modelPath,
				mmprojPath: "/path/to/model.mmproj",
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--ctx-size", "4096",
				"--mmproj", "/path/to/model.mmproj",
			),
		},
		{
			name: "reasoning budget from backend config",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				LlamaCpp: &inference.LlamaCppConfig{
					ReasoningBudget: int64ptr(1024),
				},
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--reasoning-budget", "1024",
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "reasoning budget with negative value (unlimited)",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				LlamaCpp: &inference.LlamaCppConfig{
					ReasoningBudget: int64ptr(-1),
				},
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--reasoning-budget", "-1",
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "nil LlamaCpp config (no reasoning budget)",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				LlamaCpp: nil,
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "LlamaCpp config with nil reasoning budget",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				LlamaCpp: &inference.LlamaCppConfig{
					ReasoningBudget: nil,
				},
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--ctx-size", "4096",
				"--jinja",
			),
		},
		{
			name: "reasoning budget with context size",
			mode: inference.BackendModeCompletion,
			bundle: &fakeBundle{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				ContextSize: 8192,
				LlamaCpp: &inference.LlamaCppConfig{
					ReasoningBudget: int64ptr(2048),
				},
			},
			expected: append(slices.Clone(baseArgs),
				"--model", modelPath,
				"--host", socket,
				"--reasoning-budget", "2048",
				"--ctx-size", "8192",
				"--jinja",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := config.GetArgs(tt.bundle, socket, tt.mode, tt.config)
			if err != nil {
				t.Errorf("GetArgs() error = %v", err)
			}

			// Check that all expected arguments are present and in the correct order
			expectedIndex := 0
			for i := 0; i < len(args); i++ {
				if expectedIndex >= len(tt.expected) {
					t.Errorf("Unexpected extra argument: %s", args[i])
					continue
				}

				if args[i] != tt.expected[expectedIndex] {
					t.Errorf("Expected argument %s at position %d, got %s", tt.expected[expectedIndex], i, args[i])
					continue
				}

				// If this is a flag that takes a value, check the next argument
				if i+1 < len(args) && (args[i] == "-ngl" || args[i] == "--model" || args[i] == "--host") {
					expectedIndex++
					if args[i+1] != tt.expected[expectedIndex] {
						t.Errorf("Expected value %s for flag %s, got %s", tt.expected[expectedIndex], args[i], args[i+1])
					}
					i++ // Skip the value in the next iteration
				}
				expectedIndex++
			}

			if expectedIndex != len(tt.expected) {
				t.Errorf("Missing expected arguments. Got %d arguments, expected %d", expectedIndex, len(tt.expected))
			}
		})
	}
}

func TestContainsArg(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		arg      string
		expected bool
	}{
		{
			name:     "argument exists",
			args:     []string{"--arg1", "--arg2", "--arg3"},
			arg:      "--arg2",
			expected: true,
		},
		{
			name:     "argument does not exist",
			args:     []string{"--arg1", "--arg2", "--arg3"},
			arg:      "--arg4",
			expected: false,
		},
		{
			name:     "empty args slice",
			args:     []string{},
			arg:      "--arg1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsArg(tt.args, tt.arg)
			if result != tt.expected {
				t.Errorf("containsArg(%v, %s) = %v, want %v", tt.args, tt.arg, result, tt.expected)
			}
		})
	}
}

var _ types.ModelBundle = &fakeBundle{}

type fakeBundle struct {
	ggufPath     string
	config       types.Config
	templatePath string
	mmprojPath   string
}

func (f *fakeBundle) ChatTemplatePath() string {
	return f.templatePath
}

func (f *fakeBundle) RootDir() string {
	panic("shouldn't be called")
}

func (f *fakeBundle) GGUFPath() string {
	return f.ggufPath
}

func (f *fakeBundle) MMPROJPath() string {
	return f.mmprojPath
}

func (f *fakeBundle) SafetensorsPath() string {
	return ""
}

func (f *fakeBundle) RuntimeConfig() types.Config {
	return f.config
}

func uint64ptr(n uint64) *uint64 {
	return &n
}

func int64ptr(n int64) *int64 {
	return &n
}
