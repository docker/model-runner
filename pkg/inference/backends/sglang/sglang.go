package sglang

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
	Name      = "sglang"
	sglangDir = "/opt/sglang-env"
)

var (
	ErrNotImplemented = errors.New("not implemented")
	ErrSGLangNotFound = errors.New("sglang package not installed")
	ErrPythonNotFound = errors.New("python3 not found in PATH")
)

// sglang is the SGLang-based backend implementation.
type sglang struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the SGLang server process.
	serverLog logging.Logger
	// config is the configuration for the SGLang backend.
	config *Config
	// status is the state in which the SGLang backend is in.
	status string
	// pythonPath is the path to the python3 binary.
	pythonPath string
	// customPythonPath is an optional custom path to the python3 binary.
	customPythonPath string
}

// New creates a new SGLang-based backend.
// customPythonPath is an optional path to a custom python3 binary; if empty, the default path is used.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config, customPythonPath string) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultSGLangConfig()
	}

	return &sglang{
		log:              log,
		modelManager:     modelManager,
		serverLog:        serverLog,
		config:           conf,
		status:           inference.FormatNotInstalled(""),
		customPythonPath: customPythonPath,
	}, nil
}

// Name implements inference.Backend.Name.
func (s *sglang) Name() string {
	return Name
}

func (s *sglang) UsesExternalModelManagement() bool {
	return false
}

// UsesTCP implements inference.Backend.UsesTCP.
// SGLang only supports TCP, not Unix sockets.
func (s *sglang) UsesTCP() bool {
	return true
}

func (s *sglang) Install(_ context.Context, _ *http.Client) error {
	if !platform.SupportsSGLang() {
		s.status = inference.FormatNotInstalled(inference.DetailOnlyLinux)
		return ErrNotImplemented
	}

	var pythonPath string

	// Use custom python path if specified
	if s.customPythonPath != "" {
		pythonPath = s.customPythonPath
	} else {
		venvPython := filepath.Join(sglangDir, "bin", "python3")
		pythonPath = venvPython

		if _, err := os.Stat(venvPython); err != nil {
			// Fall back to system Python
			systemPython, err := exec.LookPath("python3")
			if err != nil {
				s.status = inference.FormatError(inference.DetailPythonNotFound)
				return ErrPythonNotFound
			}
			pythonPath = systemPython
		}
	}

	s.pythonPath = pythonPath

	// Check if sglang is installed
	if err := s.pythonCmd("-c", "import sglang").Run(); err != nil {
		s.status = inference.FormatNotInstalled(inference.DetailPackageNotInstalled)
		s.log.Warnf("sglang package not found. Install with: uv pip install sglang")
		return ErrSGLangNotFound
	}

	// Get version
	output, err := s.pythonCmd("-c", "import sglang; print(sglang.__version__)").Output()
	if err != nil {
		s.log.Warnf("could not get sglang version: %v", err)
		s.status = inference.FormatRunning(inference.DetailVersionUnknown)
	} else {
		s.status = inference.FormatRunning(fmt.Sprintf("sglang %s", strings.TrimSpace(string(output))))
	}

	return nil
}

func (s *sglang) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	if !platform.SupportsSGLang() {
		s.log.Warn("sglang backend is not yet supported")
		return ErrNotImplemented
	}

	bundle, err := s.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	args, err := s.config.GetArgs(bundle, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get SGLang arguments: %w", err)
	}

	// Add served model name and weight version
	if model != "" {
		// SGLang 0.5.6+ doesn't allow colons in served-model-name (reserved for LoRA syntax)
		// Replace colons with underscores to sanitize the model name
		sanitizedModel := strings.ReplaceAll(model, ":", "_")
		args = append(args, "--served-model-name", sanitizedModel)
	}
	if modelRef != "" {
		args = append(args, "--weight-version", modelRef)
	}

	if s.pythonPath == "" {
		return fmt.Errorf("sglang: python runtime not configured; did you forget to call Install?")
	}

	sandboxPath := ""
	if _, err := os.Stat(sglangDir); err == nil {
		sandboxPath = sglangDir
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "SGLang",
		Socket:          socket,
		BinaryPath:      s.pythonPath,
		SandboxPath:     sandboxPath,
		SandboxConfig:   "",
		Args:            args,
		Logger:          s.log,
		ServerLogWriter: s.serverLog.Writer(),
	})
}

func (s *sglang) Status() string {
	return s.status
}

func (s *sglang) GetDiskUsage() (int64, error) {
	// Check if Docker installation exists
	if _, err := os.Stat(sglangDir); err == nil {
		size, err := diskusage.Size(sglangDir)
		if err != nil {
			return 0, fmt.Errorf("error while getting sglang dir size: %w", err)
		}
		return size, nil
	}
	// Python installation doesn't have a dedicated installation directory
	// It's installed via pip in the system Python environment
	return 0, nil
}

func (s *sglang) GetRequiredMemoryForModel(_ context.Context, _ string, _ *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	if !platform.SupportsSGLang() {
		return inference.RequiredMemory{}, ErrNotImplemented
	}

	return inference.RequiredMemory{
		RAM:  1,
		VRAM: 1,
	}, nil
}

// pythonCmd creates an exec.Cmd that runs python with the given arguments.
// It uses the configured pythonPath if available, otherwise falls back to "python3".
func (s *sglang) pythonCmd(args ...string) *exec.Cmd {
	pythonBinary := "python3"
	if s.pythonPath != "" {
		pythonBinary = s.pythonPath
	}
	return exec.Command(pythonBinary, args...)
}
