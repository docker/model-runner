package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/model-runner/pkg/inference"
)

type MemoryEstimator interface {
	SetDefaultBackend(MemoryEstimatorBackend)
	GetRequiredMemoryForModel(context.Context, string, *inference.BackendConfiguration) (*inference.RequiredMemory, error)
	HaveSufficientMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (bool, error)
}

type MemoryEstimatorBackend interface {
	GetRequiredMemoryForModel(context.Context, string, *inference.BackendConfiguration) (*inference.RequiredMemory, error)
}

type memoryEstimator struct {
	systemMemoryInfo SystemMemoryInfo
	defaultBackend   MemoryEstimatorBackend
}

func NewEstimator(systemMemoryInfo SystemMemoryInfo) MemoryEstimator {
	return &memoryEstimator{systemMemoryInfo: systemMemoryInfo}
}

func (m *memoryEstimator) SetDefaultBackend(backend MemoryEstimatorBackend) {
	m.defaultBackend = backend
}

func (m *memoryEstimator) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (*inference.RequiredMemory, error) {
	if m.defaultBackend == nil {
		return nil, errors.New("default backend not configured")
	}

	return m.defaultBackend.GetRequiredMemoryForModel(ctx, model, config)
}

func (m *memoryEstimator) HaveSufficientMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (bool, error) {
	req, err := m.GetRequiredMemoryForModel(ctx, model, config)
	if err != nil {
		return false, fmt.Errorf("estimating required memory for model: %w", err)
	}
	return m.systemMemoryInfo.HaveSufficientMemory(*req), nil
}
