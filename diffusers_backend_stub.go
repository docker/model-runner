//go:build nodiffusers

package main

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func initDiffusersBackend(log *logrus.Logger, modelManager *models.Manager, customPythonPath string) (inference.Backend, error) {
	return nil, nil // Diffusers backend is disabled
}

func registerDiffusersBackend(backends map[string]inference.Backend, backend inference.Backend) {
	// Diffusers backend is disabled, do nothing
}
