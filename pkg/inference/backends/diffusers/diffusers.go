package diffusers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "diffusers"
	// diffusersDir is the default installation directory in Docker containers.
	diffusersDir = "/opt/diffusers-env"
)

// ErrorNotFound indicates that the diffusers Python environment was not found.
var ErrorNotFound = errors.New("diffusers Python environment not found")

// diffusersBackend is the diffusers-based backend implementation for image generation.
type diffusersBackend struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the diffusers server process.
	serverLog logging.Logger
	// config is the configuration for the diffusers backend.
	config *Config
	// status is the state in which the diffusers backend is in.
	status string
	// pythonPath is the path to the Python interpreter to use.
	pythonPath string
}

// New creates a new diffusers-based backend for image generation.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config) (inference.Backend, error) {
	if conf == nil {
		conf = NewDefaultConfig()
	}

	return &diffusersBackend{
		log:          log,
		modelManager: modelManager,
		serverLog:    serverLog,
		config:       conf,
		status:       "not installed",
	}, nil
}

// Name implements inference.Backend.Name.
func (d *diffusersBackend) Name() string {
	return Name
}

// UsesExternalModelManagement implements inference.Backend.UsesExternalModelManagement.
func (d *diffusersBackend) UsesExternalModelManagement() bool {
	return false
}

// UsesTCP implements inference.Backend.UsesTCP.
func (d *diffusersBackend) UsesTCP() bool {
	return false
}

// Install implements inference.Backend.Install.
func (d *diffusersBackend) Install(_ context.Context, _ *http.Client) error {
	if !platform.SupportsDiffusers() {
		d.status = "not supported on this platform"
		return errors.New("diffusers is not supported on this platform")
	}

	// Try container path first
	containerPython := filepath.Join(diffusersDir, "bin", "python3")
	if _, err := os.Stat(containerPython); err == nil {
		d.pythonPath = containerPython
		return nil
	}

	// Try system Python with diffusers installed
	systemPython, err := d.findSystemPython()
	if err != nil {
		d.status = ErrorNotFound.Error()
		return ErrorNotFound
	}

	d.pythonPath = systemPython
	return nil
}

// findSystemPython looks for a Python installation with diffusers available.
func (d *diffusersBackend) findSystemPython() (string, error) {
	pythonCandidates := []string{"python3", "python"}

	// On macOS, also check common homebrew paths
	if runtime.GOOS == "darwin" {
		pythonCandidates = append([]string{
			"/opt/homebrew/bin/python3",
			"/usr/local/bin/python3",
		}, pythonCandidates...)
	}

	for _, python := range pythonCandidates {
		pythonPath, err := exec.LookPath(python)
		if err != nil {
			continue
		}

		return pythonPath, nil
	}

	return "", ErrorNotFound
}

// Run implements inference.Backend.Run.
func (d *diffusersBackend) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	if d.pythonPath == "" {
		return ErrorNotFound
	}

	if mode != inference.BackendModeImageGeneration {
		return fmt.Errorf("diffusers backend only supports image generation mode, got %s", mode.String())
	}

	bundle, err := d.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	args, err := d.config.GetArgs(bundle, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get diffusers arguments: %w", err)
	}

	// Add model name arguments
	args = append(args, "--served-model-name", model, modelRef)

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "diffusers",
		Socket:          socket,
		BinaryPath:      d.pythonPath,
		SandboxPath:     filepath.Dir(d.pythonPath),
		SandboxConfig:   "",
		Args:            args,
		Logger:          d.log,
		ServerLogWriter: d.serverLog.Writer(),
	})
}

// Status implements inference.Backend.Status.
func (d *diffusersBackend) Status() string {
	return d.status
}

// GetDiskUsage implements inference.Backend.GetDiskUsage.
func (d *diffusersBackend) GetDiskUsage() (int64, error) {
	// Check if we're using the container installation
	if _, err := os.Stat(diffusersDir); err == nil {
		size, err := diskusage.Size(diffusersDir)
		if err != nil {
			return 0, fmt.Errorf("error while getting store size: %w", err)
		}
		return size, nil
	}
	// For system Python, report 0 since it's not managed by us
	return 0, nil
}
