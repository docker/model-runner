package main

import (
	"os"
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/sirupsen/logrus"
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
			name:      "disallowed model arg with equals",
			llamaArgs: "--model=test.gguf",
			wantErr:   true,
		},
		{
			name:      "disallowed host arg",
			llamaArgs: "--host localhost:8080",
			wantErr:   true,
		},
		{
			name:      "disallowed host arg with equals",
			llamaArgs: "--host=localhost:8080",
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
			name:      "disallowed mmproj arg with equals",
			llamaArgs: "--mmproj=test.mmproj",
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
			// Set up environment
			if tt.llamaArgs != "" {
				os.Setenv("LLAMA_ARGS", tt.llamaArgs)
				defer os.Unsetenv("LLAMA_ARGS")
			}

			// Create a test logger that captures fatal errors
			originalLog := log
			defer func() { log = originalLog }()

			// Create a new logger that will exit with a special exit code
			testLog := logrus.New()
			var exitCode int
			testLog.ExitFunc = func(code int) {
				exitCode = code
			}
			log = testLog

			config := createLlamaCppConfigFromEnv()

			// With the new error handling, we don't exit on error, just log it
			// So we expect exitCode to always be 0 (no fatal exit)
			if exitCode != 0 {
				t.Errorf("Expected exit code 0, got %d", exitCode)
			}

			if tt.wantErr {
				// For error cases, we now return nil config instead of exiting
				if config != nil {
					t.Error("Expected nil config for error cases")
				}
			} else {
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
