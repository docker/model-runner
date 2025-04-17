package vllm

import (
	"context"
	"errors"
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "vllm"
)

// vLLM is the vLLM-based backend implementation.
type vLLM struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
}

// New creates a new vLLM-based backend.
func New(log logging.Logger, modelManager *models.Manager) (inference.Backend, error) {
	return &vLLM{
		log:          log,
		modelManager: modelManager,
	}, nil
}

// Name implements inference.Backend.Name.
func (v *vLLM) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (l *vLLM) UsesExternalModelManagement() bool {
	return false
}

// Install implements inference.Backend.Install.
func (v *vLLM) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (v *vLLM) Run(ctx context.Context, socket, model string, mode inference.BackendMode) error {
	// TODO: Implement.
	v.log.Warn("vLLM backend is not yet supported")
	return errors.New("not implemented")
}

func (v *vLLM) Status() string {
	return "not running"
}
