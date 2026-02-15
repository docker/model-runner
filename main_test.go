package main

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
)

func TestCreateLlamaCppConfigFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		llamaArgs string
		wantNil   bool
	}{
		{
			name:      "empty args",
			llamaArgs: "",
			wantNil:   true,
		},
		{
			name:      "valid args",
			llamaArgs: "--threads 4 --ctx-size 2048",
			wantNil:   false,
		},
		{
			name:      "disallowed model arg",
			llamaArgs: "--model test.gguf",
			wantNil:   false, // config is still created, error is logged
		},
		{
			name:      "disallowed host arg",
			llamaArgs: "--host localhost:8080",
			wantNil:   false,
		},
		{
			name:      "disallowed embeddings arg",
			llamaArgs: "--embeddings",
			wantNil:   false,
		},
		{
			name:      "disallowed mmproj arg",
			llamaArgs: "--mmproj test.mmproj",
			wantNil:   false,
		},
		{
			name:      "multiple disallowed args",
			llamaArgs: "--model test.gguf --host localhost:8080",
			wantNil:   false,
		},
		{
			name:      "quoted args",
			llamaArgs: "--prompt \"Hello, world!\" --threads 4",
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.llamaArgs != "" {
				t.Setenv("LLAMA_ARGS", tt.llamaArgs)
			}

			config := createLlamaCppConfigFromEnv()

			if tt.wantNil {
				if config != nil {
					t.Error("Expected nil config for empty args")
				}
				return
			}

			if config == nil {
				t.Fatal("Expected non-nil config")
			}

			llamaConfig, ok := config.(*llamacpp.Config)
			if !ok {
				t.Errorf("Expected *llamacpp.Config, got %T", config)
			}
			if llamaConfig == nil {
				t.Fatal("Expected non-nil config")
			}
			if len(llamaConfig.Args) == 0 {
				t.Error("Expected non-empty args")
			}
		})
	}
}
