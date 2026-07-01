package ovms

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "ovms"
)

var ErrOVMSNotFound = errors.New("ovms binary not found")

type ovms struct {
	log              logging.Logger
	modelManager     *models.Manager
	serverLog        logging.Logger
	status           string
	customBinaryPath string
}

func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, customBinaryPath string) (inference.Backend, error) {
	return &ovms{
		log:              log,
		modelManager:     modelManager,
		serverLog:        serverLog,
		status:           inference.FormatNotInstalled(""),
		customBinaryPath: customBinaryPath,
	}, nil
}

func (o *ovms) Name() string {
	return Name
}

func (o *ovms) UsesExternalModelManagement() bool {
	return false
}

func (o *ovms) UsesTCP() bool {
	return true
}

func (o *ovms) HealthPath() string {
	return "/v2/health/ready"
}

func (o *ovms) RewritePath(path string) string {
	if len(path) > 3 && path[:4] == "/v1/" {
		return "/v3/" + path[4:]
	}
	return path
}

func (o *ovms) Install(ctx context.Context, _ *http.Client) error {
	binary := o.binaryPath()
	if o.customBinaryPath != "" {
		o.log.Info("OVMS binary configured via OVMS_SERVER_PATH", "path", binary)
	} else if resolved, err := exec.LookPath(Name); err == nil {
		o.log.Info("OVMS binary resolved from PATH", "path", resolved)
	}
	if _, err := exec.LookPath(binary); err != nil {
		o.status = inference.FormatNotInstalled("")
		return ErrOVMSNotFound
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(checkCtx, binary, "--version").Output()
	if err != nil {
		o.log.Warn("could not get OVMS version", "error", err)
		o.status = inference.FormatRunning(inference.DetailVersionUnknown)
		return nil
	}

	versionLine := strings.TrimSpace(string(output))
	if versionLine == "" {
		o.status = inference.FormatRunning(inference.DetailVersionUnknown)
		return nil
	}

	o.status = inference.FormatRunning(versionLine)
	return nil
}

func (o *ovms) Run(ctx context.Context, socket, model string, modelRef string, _ inference.BackendMode, _ *inference.BackendConfiguration) error {
	bundle, err := o.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}
	modelPath := resolveOVMSModelPath(bundle.RootDir())

	_, port, err := net.SplitHostPort(socket)
	if err != nil {
		return fmt.Errorf("invalid backend socket address %q: %w", socket, err)
	}

	// Use the human-readable model reference for --model_name so that
	// incoming requests (which carry the original name) match.
	modelName := modelRef
	if modelName == "" {
		modelName = model
	}
	logLevel := ovmsLogLevel(o.log)

	args := []string{
		"--rest_port", port,
		"--port", "0",
		"--model_name", modelName,
		"--model_path", modelPath,
		"--task", "text_generation",
		"--log_level", logLevel,
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "OVMS",
		Socket:          socket,
		BinaryPath:      o.binaryPath(),
		SandboxPath:     filepath.Dir(o.binaryPath()),
		SandboxConfig:   "",
		Args:            args,
		Logger:          o.log,
		ServerLogWriter: logging.NewWriter(o.serverLog),
	})
}

// Uninstall implements inference.Backend.Uninstall.
func (o *ovms) Uninstall() error {
	return nil
}

func (o *ovms) Status() string {
	return o.status
}

func (o *ovms) GetDiskUsage() (int64, error) {
	return 0, nil
}

func (o *ovms) binaryPath() string {
	if o.customBinaryPath != "" {
		return o.customBinaryPath
	}
	if path, err := exec.LookPath(Name); err == nil {
		return path
	}
	// Keep command name as a last resort so error reporting remains clear.
	return Name
}

// resolveOVMSModelPath returns the path OVMS should receive via --model_path.
// Runtime bundles store model files under a dedicated "model" subdirectory.
// Fallback to the bundle root for backward compatibility if it does not exist.
func resolveOVMSModelPath(bundleRoot string) string {
	modelDir := filepath.Join(bundleRoot, "model")
	if info, err := os.Stat(modelDir); err == nil && info.IsDir() {
		return modelDir
	}
	return bundleRoot
}

func ovmsLogLevel(logger logging.Logger) string {
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		return "DEBUG"
	}
	return "INFO"
}
