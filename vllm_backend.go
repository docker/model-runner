//go:build !novllm

package main

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func initVLLMBackend(log *logrus.Logger, modelManager *models.Manager) (inference.Backend, error) {
	return vllm.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": vllm.Name}),
		nil,
	)
}

func registerVLLMBackend(backends map[string]inference.Backend, backend inference.Backend) {
	if backend != nil {
		backends[vllm.Name] = backend
	}
}
