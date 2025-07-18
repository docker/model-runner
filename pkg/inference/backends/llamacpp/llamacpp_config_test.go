package llamacpp

import (
	"errors"
	"runtime"
	"strconv"
	"testing"

	"github.com/docker/model-distribution/types"
	"github.com/docker/model-runner/pkg/inference"
)

func TestNewDefaultLlamaCppConfig(t *testing.T) {
	config := NewDefaultLlamaCppConfig()

	// Test default arguments
	if !containsArg(config.Args, "--jinja") {
		t.Error("Expected --jinja argument to be present")
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
	if config.Args[nglIndex+1] != "100" {
		t.Errorf("Expected -ngl value to be 100, got %s", config.Args[nglIndex+1])
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

	tests := []struct {
		name     string
		model    types.Model
		mode     inference.BackendMode
		config   *inference.BackendConfiguration
		expected []string
	}{
		{
			name: "completion mode",
			mode: inference.BackendModeCompletion,
			model: &fakeModel{
				ggufPath: modelPath,
			},
			expected: []string{
				"--jinja",
				"-ngl", "100",
				"--metrics",
				"--model", modelPath,
				"--host", socket,
				"--ctx-size", "4096",
			},
		},
		{
			name: "embedding mode",
			mode: inference.BackendModeEmbedding,
			model: &fakeModel{
				ggufPath: modelPath,
			},
			expected: []string{
				"--jinja",
				"-ngl", "100",
				"--metrics",
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "4096",
			},
		},
		{
			name: "context size from backend config",
			mode: inference.BackendModeEmbedding,
			model: &fakeModel{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				ContextSize: 1234,
			},
			expected: []string{
				"--jinja",
				"-ngl", "100",
				"--metrics",
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "1234", // should add this flag
			},
		},
		{
			name: "context size from model config",
			mode: inference.BackendModeEmbedding,
			model: &fakeModel{
				ggufPath: modelPath,
				config: types.Config{
					ContextSize: uint64ptr(2096),
				},
			},
			config: &inference.BackendConfiguration{
				ContextSize: 1234,
			},
			expected: []string{
				"--jinja",
				"-ngl", "100",
				"--metrics",
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "2096", // model config takes precedence
			},
		},
		{
			name: "raw flags from backend config",
			mode: inference.BackendModeEmbedding,
			model: &fakeModel{
				ggufPath: modelPath,
			},
			config: &inference.BackendConfiguration{
				RuntimeFlags: []string{"--some", "flag"},
			},
			expected: []string{
				"--jinja",
				"-ngl", "100",
				"--metrics",
				"--model", modelPath,
				"--host", socket,
				"--embeddings",
				"--ctx-size", "4096",
				"--some", "flag", // model config takes precedence
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := config.GetArgs(tt.model, socket, tt.mode, tt.config)
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

var _ types.Model = &fakeModel{}

type fakeModel struct {
	ggufPath string
	config   types.Config
}

func (f *fakeModel) MMPROJPath() (string, error) {
	return "", errors.New("not found")
}

func (f *fakeModel) ID() (string, error) {
	panic("shouldn't be called")
}

func (f *fakeModel) GGUFPath() (string, error) {
	return f.ggufPath, nil
}

func (f *fakeModel) Config() (types.Config, error) {
	return f.config, nil
}

func (f *fakeModel) Tags() []string {
	panic("shouldn't be called")
}

func (f fakeModel) Descriptor() (types.Descriptor, error) {
	panic("shouldn't be called")
}

func uint64ptr(n uint64) *uint64 {
	return &n
}
