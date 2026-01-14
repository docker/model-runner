//go:build !nodiffusers

package main

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func initDiffusersBackend(log *logrus.Logger, modelManager *models.Manager, customPythonPath string) (inference.Backend, error) {
	return diffusers.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": diffusers.Name}),
		nil,
		customPythonPath,
	)
}

func registerDiffusersBackend(backends map[string]inference.Backend, backend inference.Backend) {
	if backend != nil {
		backends[diffusers.Name] = backend
	}
}
