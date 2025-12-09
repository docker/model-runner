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
}

// New creates a new SGLang-based backend.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultSGLangConfig()
	}

	return &sglang{
		log:          log,
		modelManager: modelManager,
		serverLog:    serverLog,
		config:       conf,
		status:       "not installed",
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
		return ErrNotImplemented
	}

	venvPython := filepath.Join(sglangDir, "bin", "python3")
	pythonPath := venvPython

	if _, err := os.Stat(venvPython); err != nil {
		// Fall back to system Python
		systemPython, err := exec.LookPath("python3")
		if err != nil {
			s.status = ErrPythonNotFound.Error()
			return ErrPythonNotFound
		}
		pythonPath = systemPython
	}

	s.pythonPath = pythonPath

	// Check if sglang is installed
	if err := s.pythonCmd("-c", "import sglang").Run(); err != nil {
		s.status = "sglang package not installed"
		s.log.Warnf("sglang package not found. Install with: uv pip install sglang[all]")
		return ErrSGLangNotFound
	}

	// Get version
	output, err := s.pythonCmd("-c", "import sglang; print(sglang.__version__)").Output()
	if err != nil {
		s.log.Warnf("could not get sglang version: %v", err)
		s.status = "running sglang version: unknown"
	} else {
		s.status = fmt.Sprintf("running sglang version: %s", strings.TrimSpace(string(output)))
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
	// TODO: Implement accurate memory estimation based on model size and SGLang's memory requirements.
	// Returning an error prevents the scheduler from making incorrect decisions based
	// on placeholder values.
	return inference.RequiredMemory{}, ErrNotImplemented
}

// pythonCmd creates an exec.Cmd that runs python3 with the given arguments.
func (s *sglang) pythonCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("python3", args...)

	// Override the actual binary path if we discovered a specific interpreter.
	if s.pythonPath != "" && s.pythonPath != "python3" {
		cmd.Path = s.pythonPath
	}

	return cmd
}
