package mlx

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
	Name = "mlx"
)

// mlx is the MLX-based backend implementation.
type mlx struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
}

// New creates a new MLX-based backend.
func New(log logging.Logger, modelManager *models.Manager) (inference.Backend, error) {
	return &mlx{
		log:          log,
		modelManager: modelManager,
	}, nil
}

// Name implements inference.Backend.Name.
func (m *mlx) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (l *mlx) UsesExternalModelManagement() bool {
	return false
}

// Install implements inference.Backend.Install.
func (m *mlx) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (m *mlx) Run(ctx context.Context, socket, model string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	// TODO: Implement.
	m.log.Warn("MLX backend is not yet supported")
	return errors.New("not implemented")
}

func (m *mlx) Status() string {
	return "not running"
}

func (m *mlx) GetDiskUsage() (int64, error) {
	return 0, nil
}

func (m *mlx) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (*inference.RequiredMemory, error) {
	return nil, errors.New("not implemented")
}
