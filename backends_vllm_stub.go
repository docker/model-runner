//go:build novllm

package main

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func initVLLMBackend(log *logrus.Logger, modelManager *models.Manager) (inference.Backend, error) {
	return nil, nil
}

func registerVLLMBackend(backends map[string]inference.Backend, backend inference.Backend) {
	// No-op when vLLM is disabled
}