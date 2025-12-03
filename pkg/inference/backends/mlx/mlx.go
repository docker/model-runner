package mlx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "mlx"
)

var ErrStatusNotFound = errors.New("Python or mlx-lm not found")

// mlx is the MLX-based backend implementation.
type mlx struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the MLX server process.
	serverLog logging.Logger
	// config is the configuration for the MLX backend.
	config *Config
	// status is the state in which the MLX backend is in.
	status string
	// pythonPath is the path to the python3 binary.
	pythonPath string
}

// New creates a new MLX-based backend.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultMLXConfig()
	}

	return &mlx{
		log:          log,
		modelManager: modelManager,
		serverLog:    serverLog,
		config:       conf,
		status:       "not installed",
	}, nil
}

// Name implements inference.Backend.Name.
func (m *mlx) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (m *mlx) UsesExternalModelManagement() bool {
	return false
}

// UsesTCP implements inference.Backend.UsesTCP.
func (m *mlx) UsesTCP() bool {
	return false
}

// Install implements inference.Backend.Install.
func (m *mlx) Install(ctx context.Context, httpClient *http.Client) error {
	if !platform.SupportsMLX() {
		return errors.New("MLX is only available on macOS ARM64")
	}

	// Check if Python 3 is available
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		m.status = ErrStatusNotFound.Error()
		return ErrStatusNotFound
	}

	// Store the python path for later use
	m.pythonPath = pythonPath

	// Check if mlx-lm package is installed by attempting to import it
	cmd := exec.CommandContext(ctx, pythonPath, "-c", "import mlx_lm")
	if err := cmd.Run(); err != nil {
		m.status = "mlx-lm package not installed"
		m.log.Warnf("mlx-lm package not found. Install with: uv pip install mlx-lm")
		return fmt.Errorf("mlx-lm package not installed: %w", err)
	}

	// Get MLX version
	cmd = exec.CommandContext(ctx, pythonPath, "-c", "import mlx; print(mlx.__version__)")
	output, err := cmd.Output()
	if err != nil {
		m.log.Warnf("could not get MLX version: %v", err)
		m.status = "running MLX version: unknown"
	} else {
		m.status = fmt.Sprintf("running MLX version: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// Run implements inference.Backend.Run.
func (m *mlx) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	bundle, err := m.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	args, err := m.config.GetArgs(bundle, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get MLX arguments: %w", err)
	}

	// Add served model name
	args = append(args, "--served-model-name", model, modelRef)

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "MLX",
		Socket:          socket,
		BinaryPath:      m.pythonPath,
		SandboxPath:     "",
		SandboxConfig:   "",
		Args:            args,
		Logger:          m.log,
		ServerLogWriter: m.serverLog.Writer(),
	})
}

func (m *mlx) Status() string {
	return m.status
}

func (m *mlx) GetDiskUsage() (int64, error) {
	// MLX doesn't have a dedicated installation directory
	// It's installed via pip in the system Python environment
	return 0, nil
}

func (m *mlx) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	// TODO: Implement accurate memory estimation based on model size.
	// MLX runs on unified memory architecture (Apple Silicon), so memory estimation
	// will need to account for the unified nature of RAM and VRAM on Apple Silicon.
	// Returning an error prevents the scheduler from making incorrect decisions based
	// on placeholder values.
	return inference.RequiredMemory{}, errors.New("not implemented")
}
