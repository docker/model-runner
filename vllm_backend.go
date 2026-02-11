//go:build !novllm

package main

import (
	"log/slog"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/models"
)

func initVLLMBackend(log *slog.Logger, modelManager *models.Manager, customBinaryPath string) (inference.Backend, error) {
	return vllm.New(
		log,
		modelManager,
		log.With("component", vllm.Name),
		nil,
		customBinaryPath,
	)
}

func registerVLLMBackend(backends map[string]inference.Backend, backend inference.Backend) {
	if backend != nil {
		backends[vllm.Name] = backend
	}
}
