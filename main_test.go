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

			cfg, err := createLlamaCppConfigFromEnv()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.llamaArgs == "" {
				if cfg != nil {
					t.Error("expected nil config for empty args")
				}
			} else {
				llamaConfig, ok := cfg.(*llamacpp.Config)
				if !ok {
					t.Fatalf("expected *llamacpp.Config, got %T", cfg)
				}
				if llamaConfig == nil {
					t.Fatal("expected non-nil config")
				}
				if len(llamaConfig.Args) == 0 {
					t.Error("expected non-empty args")
				}
			}
		})
	}
}
