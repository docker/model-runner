package sglang

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	Name              = "sglang"
	sglangDir         = "/opt/sglang-env"
	sglangVersionFile = "/opt/sglang-env/version"
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

	if err := s.initFromDocker(); err == nil {
		s.log.Infof("installed sglang from docker: %s", s.status)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to check SGLang binary: %w", err)
	}

	if err := s.initFromHost(); err != nil {
		return err
	}
	s.log.Infof("installed sglang from host: %s", s.status)
	return nil
}

func (s *sglang) initFromDocker() error {
	sglangBinaryPath := s.binaryPath()

	if _, err := os.Stat(sglangBinaryPath); err != nil {
		return err
	}

	versionBytes, err := os.ReadFile(sglangVersionFile)
	if err != nil {
		s.log.Warnf("could not get sglang version: %v", err)
		s.status = "running sglang version: unknown"
		return nil
	}

	s.status = fmt.Sprintf(
		"running sglang version: %s",
		strings.TrimSpace(string(versionBytes)),
	)

	return nil
}

func (s *sglang) initFromHost() error {
	venvPython := filepath.Join(sglangDir, "bin", "python3")
	pythonPath := venvPython

	if _, err := os.Stat(venvPython); err != nil {
		systemPython, err := exec.LookPath("python3")
		if err != nil {
			s.status = ErrPythonNotFound.Error()
			return ErrPythonNotFound
		}
		pythonPath = systemPython
	}

	s.pythonPath = pythonPath

	if err := exec.Command(pythonPath, "-c", "import sglang").Run(); err != nil {
		s.status = "sglang package not installed"
		s.log.Warnf("sglang package not found. Install with: uv pip install sglang[all]")
		return ErrSGLangNotFound
	}

	output, err := exec.Command(pythonPath, "-c", "import sglang; print(sglang.__version__)").Output()
	if err != nil {
		s.log.Warnf("could not get sglang version: %v", err)
		s.status = "running sglang version: unknown"
		return nil
	}

	s.status = fmt.Sprintf("running sglang version: %s", strings.TrimSpace(string(output)))

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
		args = append(args, "--served-model-name", model)
	}
	if modelRef != "" {
		args = append(args, "--weight-version", modelRef)
	}

	// Determine binary path - use Docker installation if available, otherwise use Python
	binaryPath := s.binaryPath()
	sandboxPath := sglangDir
	if _, err := os.Stat(binaryPath); errors.Is(err, fs.ErrNotExist) {
		// Use Python installation
		if s.pythonPath == "" {
			return fmt.Errorf("sglang: no docker binary at %s and no python runtime configured; did you forget to call Install?", binaryPath)
		}
		binaryPath = s.pythonPath
		sandboxPath = ""
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "SGLang",
		Socket:          socket,
		BinaryPath:      binaryPath,
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

func (s *sglang) binaryPath() string {
	return filepath.Join(sglangDir, "sglang")
}
