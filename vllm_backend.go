//go:build !novllm

package main

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/routing"
	"github.com/sirupsen/logrus"
)

func vllmBackendDefs(log *logrus.Logger, customBinaryPath string) []routing.BackendDef {
	return []routing.BackendDef{{
		Name: vllm.Name,
		Init: func(mm *models.Manager) (inference.Backend, error) {
			return vllm.New(log, mm, log.WithFields(logrus.Fields{"component": vllm.Name}), nil, customBinaryPath)
		},
	}}
}
