package main

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
)

func TestCreateLlamaCppConfigFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		llamaArgs string
		wantErr   bool
	}{
		{
			name:      "empty args",
			llamaArgs: "",
			wantErr:   false,
		},
		{
			name:      "valid args",
			llamaArgs: "--threads 4 --ctx-size 2048",
			wantErr:   false,
		},
		{
			name:      "disallowed model arg",
			llamaArgs: "--model test.gguf",
			wantErr:   true,
		},
		{
			name:      "disallowed host arg",
			llamaArgs: "--host localhost:8080",
			wantErr:   true,
		},
		{
			name:      "disallowed embeddings arg",
			llamaArgs: "--embeddings",
			wantErr:   true,
		},
		{
			name:      "disallowed mmproj arg",
			llamaArgs: "--mmproj test.mmproj",
			wantErr:   true,
		},
		{
			name:      "multiple disallowed args",
			llamaArgs: "--model test.gguf --host localhost:8080",
			wantErr:   true,
		},
		{
			name:      "quoted args",
			llamaArgs: "--prompt \"Hello, world!\" --threads 4",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.llamaArgs != "" {
				t.Setenv("LLAMA_ARGS", tt.llamaArgs)
			}

			// Override exitFunc to capture exit calls instead of actually exiting
			originalExitFunc := exitFunc
			defer func() { exitFunc = originalExitFunc }()

			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			config := createLlamaCppConfigFromEnv()

			if tt.wantErr {
				if exitCode != 1 {
					t.Errorf("Expected exit code 1, got %d", exitCode)
				}
			} else {
				if exitCode != 0 {
					t.Errorf("Expected exit code 0, got %d", exitCode)
				}
				if tt.llamaArgs == "" {
					if config != nil {
						t.Error("Expected nil config for empty args")
					}
				} else {
					llamaConfig, ok := config.(*llamacpp.Config)
					if !ok {
						t.Fatalf("Expected *llamacpp.Config, got %T", config)
					}
					if llamaConfig == nil {
						t.Fatal("Expected non-nil config")
					}
					if len(llamaConfig.Args) == 0 {
						t.Error("Expected non-empty args")
					}
				}
			}
		})
	}
}
