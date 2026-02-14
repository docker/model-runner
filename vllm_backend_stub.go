//go:build novllm

package main

import (
	"log/slog"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
)

func initVLLMBackend(log *slog.Logger, modelManager *models.Manager, customBinaryPath string) (inference.Backend, error) {
	return nil, nil
}

func registerVLLMBackend(backends map[string]inference.Backend, backend inference.Backend) {
	// No-op when VLLM is disabled
}
