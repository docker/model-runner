package diffusers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name         = "diffusers"
	diffusersDir = "/opt/diffusers-env"
)

var (
	ErrNotImplemented    = errors.New("not implemented")
	ErrDiffusersNotFound = errors.New("diffusers package not installed")
	ErrPythonNotFound    = errors.New("python3 not found in PATH")
)

// diffusers is the diffusers-based backend implementation for image generation.
type diffusers struct {
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
	// pythonPath is the path to the python3 binary.
	pythonPath string
	// customPythonPath is an optional custom path to the python3 binary.
	customPythonPath string
}

// New creates a new diffusers-based backend for image generation.
// customPythonPath is an optional path to a custom python3 binary; if empty, the default path is used.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config, customPythonPath string) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultConfig()
	}

	return &diffusers{
		log:              log,
		modelManager:     modelManager,
		serverLog:        serverLog,
		config:           conf,
		status:           "not installed",
		customPythonPath: customPythonPath,
	}, nil
}

// Name implements inference.Backend.Name.
func (d *diffusers) Name() string {
	return Name
}

// UsesExternalModelManagement implements inference.Backend.UsesExternalModelManagement.
// Diffusers uses the shared model manager but also supports loading models directly from HuggingFace.
func (d *diffusers) UsesExternalModelManagement() bool {
	return true // For now, we'll use external model management (HuggingFace downloads)
}

// UsesTCP implements inference.Backend.UsesTCP.
// Diffusers uses TCP for communication, like SGLang.
func (d *diffusers) UsesTCP() bool {
	return true
}

// Install implements inference.Backend.Install.
func (d *diffusers) Install(_ context.Context, _ *http.Client) error {
	if !platform.SupportsDiffusers() {
		return ErrNotImplemented
	}

	var pythonPath string

	// Use custom python path if specified
	if d.customPythonPath != "" {
		pythonPath = d.customPythonPath
	} else {
		venvPython := filepath.Join(diffusersDir, "bin", "python3")
		pythonPath = venvPython

		if _, err := os.Stat(venvPython); err != nil {
			// Fall back to system Python
			systemPython, err := exec.LookPath("python3")
			if err != nil {
				d.status = ErrPythonNotFound.Error()
				return ErrPythonNotFound
			}
			pythonPath = systemPython
		}
	}

	d.pythonPath = pythonPath

	// Check if diffusers is installed
	if err := d.pythonCmd("-c", "import diffusers").Run(); err != nil {
		d.status = "diffusers package not installed"
		d.log.Warnf("diffusers package not found. Install with: uv pip install diffusers torch")
		return ErrDiffusersNotFound
	}

	// Get version
	output, err := d.pythonCmd("-c", "import diffusers; print(diffusers.__version__)").Output()
	if err != nil {
		d.log.Warnf("could not get diffusers version: %v", err)
		d.status = "running diffusers version: unknown"
	} else {
		d.status = fmt.Sprintf("running diffusers version: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// Run implements inference.Backend.Run.
func (d *diffusers) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	if !platform.SupportsDiffusers() {
		d.log.Warn("diffusers backend is not yet supported on this platform")
		return ErrNotImplemented
	}

	// For diffusers, we support image generation mode
	if mode != inference.BackendModeImageGeneration {
		return fmt.Errorf("diffusers backend only supports image-generation mode, got %s", mode)
	}

	args, err := d.config.GetArgs(model, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get diffusers arguments: %w", err)
	}

	// Add served model name
	if model != "" {
		// Replace colons with underscores to sanitize the model name
		sanitizedModel := strings.ReplaceAll(model, ":", "_")
		args = append(args, "--served-model-name", sanitizedModel)
	}

	if d.pythonPath == "" {
		return fmt.Errorf("diffusers: python runtime not configured; did you forget to call Install?")
	}

	sandboxPath := ""
	if _, err := os.Stat(diffusersDir); err == nil {
		sandboxPath = diffusersDir
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "Diffusers",
		Socket:          socket,
		BinaryPath:      d.pythonPath,
		SandboxPath:     sandboxPath,
		SandboxConfig:   "",
		Args:            args,
		Logger:          d.log,
		ServerLogWriter: d.serverLog.Writer(),
	})
}

// Status implements inference.Backend.Status.
func (d *diffusers) Status() string {
	return d.status
}

// GetDiskUsage implements inference.Backend.GetDiskUsage.
func (d *diffusers) GetDiskUsage() (int64, error) {
	// Check if Docker installation exists
	if _, err := os.Stat(diffusersDir); err == nil {
		size, err := diskusage.Size(diffusersDir)
		if err != nil {
			return 0, fmt.Errorf("error while getting diffusers dir size: %w", err)
		}
		return size, nil
	}
	// Python installation doesn't have a dedicated installation directory
	// It's installed via pip in the system Python environment
	return 0, nil
}

// GetRequiredMemoryForModel returns the estimated memory requirements for a model.
func (d *diffusers) GetRequiredMemoryForModel(_ context.Context, _ string, _ *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	if !platform.SupportsDiffusers() {
		return inference.RequiredMemory{}, ErrNotImplemented
	}

	// Stable Diffusion models typically require significant VRAM
	// SD 1.5: ~4GB VRAM, SD 2.1: ~5GB VRAM, SDXL: ~8GB VRAM
	return inference.RequiredMemory{
		RAM:  4 * 1024 * 1024 * 1024, // 4GB RAM
		VRAM: 6 * 1024 * 1024 * 1024, // 6GB VRAM (average estimate)
	}, nil
}

// pythonCmd creates an exec.Cmd that runs python with the given arguments.
// It uses the configured pythonPath if available, otherwise falls back to "python3".
func (d *diffusers) pythonCmd(args ...string) *exec.Cmd {
	pythonBinary := "python3"
	if d.pythonPath != "" {
		pythonBinary = d.pythonPath
	}
	return exec.Command(pythonBinary, args...)
}
