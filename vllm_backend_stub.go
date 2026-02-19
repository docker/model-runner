//go:build novllm

package main

import (
	"github.com/docker/model-runner/pkg/routing"
	"github.com/sirupsen/logrus"
)

func vllmBackendDefs(_ *logrus.Logger, _ string) []routing.BackendDef {
	return nil
}
