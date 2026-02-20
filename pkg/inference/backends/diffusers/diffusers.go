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
	"strings"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/internal/dockerhub"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name              = "diffusers"
	defaultInstallDir = ".docker/model-runner/diffusers"
	// diffusersVersion is the diffusers release tag to download from Docker Hub.
	diffusersVersion = "v0.1.0-20260216-000000"
)

var (
	ErrNoDDUFFile = errors.New("no DDUF file found in model bundle")
	// ErrPlatformNotSupported indicates the platform is not supported.
	ErrPlatformNotSupported = errors.New("diffusers is not available on this platform")
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
	// pythonPath is the path to the bundled python3 binary.
	pythonPath string
	// customPythonPath is an optional custom path to a python3 binary.
	customPythonPath string
	// installDir is the directory where diffusers is installed.
	installDir string
}

// New creates a new diffusers-based backend for image generation.
// customPythonPath is an optional path to a custom python3 binary; if empty, the default installation is used.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config, customPythonPath string) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultConfig()
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	installDir := filepath.Join(homeDir, defaultInstallDir)

	return &diffusers{
		log:              log,
		modelManager:     modelManager,
		serverLog:        serverLog,
		config:           conf,
		status:           inference.FormatNotInstalled(""),
		customPythonPath: customPythonPath,
		installDir:       installDir,
	}, nil
}

// Name implements inference.Backend.Name.
func (d *diffusers) Name() string {
	return Name
}

// UsesExternalModelManagement implements inference.Backend.UsesExternalModelManagement.
// Diffusers uses the shared model manager with bundled DDUF files.
func (d *diffusers) UsesExternalModelManagement() bool {
	return false
}

// UsesTCP implements inference.Backend.UsesTCP.
// Diffusers uses TCP for communication.
func (d *diffusers) UsesTCP() bool {
	return true
}

// Install implements inference.Backend.Install.
func (d *diffusers) Install(ctx context.Context, httpClient *http.Client) error {
	if !platform.SupportsDiffusers() {
		return ErrPlatformNotSupported
	}

	if d.customPythonPath != "" {
		d.pythonPath = d.customPythonPath
		return d.verifyInstallation(ctx)
	}

	pythonPath := filepath.Join(d.installDir, "bin", "python3")
	versionFile := filepath.Join(d.installDir, ".diffusers-version")

	// Check if already installed with correct version
	if _, err := os.Stat(pythonPath); err == nil {
		if installedVersion, err := os.ReadFile(versionFile); err == nil {
			installed := strings.TrimSpace(string(installedVersion))
			if installed == diffusersVersion || installed == "dev" {
				d.pythonPath = pythonPath
				return d.verifyInstallation(ctx)
			}
			d.log.Infof("diffusers version mismatch: installed %s, want %s", installed, diffusersVersion)
		}
	}

	d.status = "installing"
	if err := d.downloadAndExtract(ctx, httpClient); err != nil {
		return fmt.Errorf("failed to install diffusers: %w", err)
	}

	// Save version file
	if err := os.WriteFile(versionFile, []byte(diffusersVersion), 0644); err != nil {
		d.log.Warnf("failed to write version file: %v", err)
	}

	d.pythonPath = pythonPath
	return d.verifyInstallation(ctx)
}

// downloadAndExtract downloads the diffusers image from Docker Hub and extracts it.
// The image contains a self-contained Python installation with all packages pre-installed.
func (d *diffusers) downloadAndExtract(ctx context.Context, _ *http.Client) error {
	d.log.Infof("Downloading diffusers %s from Docker Hub...", diffusersVersion)

	// Create temp directory for download
	downloadDir, err := os.MkdirTemp("", "diffusers-install")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	// Pull the image
	image := fmt.Sprintf("registry-1.docker.io/docker/model-runner:diffusers-%s", diffusersVersion)
	if err := dockerhub.PullPlatform(ctx, image, filepath.Join(downloadDir, "image.tar"), runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Extract the image
	extractDir := filepath.Join(downloadDir, "extracted")
	if err := dockerhub.Extract(filepath.Join(downloadDir, "image.tar"), runtime.GOARCH, runtime.GOOS, extractDir); err != nil {
		return fmt.Errorf("failed to extract image: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(d.installDir), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	// Remove existing install dir if it exists (incomplete installation)
	if err := os.RemoveAll(d.installDir); err != nil {
		return fmt.Errorf("failed to remove existing install dir: %w", err)
	}

	d.log.Infof("Extracting self-contained Python environment...")

	// Copy the extracted self-contained Python installation directly to install dir
	// (the image contains /diffusers/ with bin/, lib/, etc.)
	diffusersDir := filepath.Join(extractDir, "diffusers")
	if err := utils.CopyDir(diffusersDir, d.installDir); err != nil {
		return fmt.Errorf("failed to copy to install dir: %w", err)
	}

	// Docker COPY strips execute permissions in OCI image layers.
	// Restore the execute bit on the bundled Python binary.
	if err := os.Chmod(filepath.Join(d.installDir, "bin", "python3"), 0755); err != nil {
		return fmt.Errorf("failed to make python3 executable: %w", err)
	}

	d.log.Infof("diffusers %s installed successfully", diffusersVersion)
	return nil
}

// verifyInstallation checks that the diffusers Python package can be imported.
// Note: d.pythonPath is not user-controlled — it is set internally by Install()
// to the bundled Python binary path, so the exec.Command usage is safe.
func (d *diffusers) verifyInstallation(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, d.pythonPath, "-c", "import diffusers") //nolint:gosec // pythonPath is set internally by Install, not user input
	if err := cmd.Run(); err != nil {
		d.status = "import failed"
		return fmt.Errorf("diffusers import failed: %w", err)
	}

	versionFile := filepath.Join(d.installDir, ".diffusers-version")
	versionBytes, err := os.ReadFile(versionFile)
	if err != nil {
		d.status = "running diffusers"
		return nil
	}
	d.status = fmt.Sprintf("running diffusers %s", strings.TrimSpace(string(versionBytes)))
	return nil
}

// Run implements inference.Backend.Run.
func (d *diffusers) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	if !platform.SupportsDiffusers() {
		d.log.Warn("diffusers backend is not yet supported on this platform")
		return ErrPlatformNotSupported
	}

	// For diffusers, we support image generation mode
	if mode != inference.BackendModeImageGeneration {
		return fmt.Errorf("diffusers backend only supports image-generation mode, got %s", mode)
	}

	// Get the model bundle to find the DDUF file path
	bundle, err := d.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model bundle for %s: %w", model, err)
	}

	// Get the DDUF file path from the bundle
	ddufPath := bundle.DDUFPath()
	if ddufPath == "" {
		return fmt.Errorf("%w: model %s", ErrNoDDUFFile, model)
	}

	d.log.Infof("Loading DDUF file from: %s", ddufPath)

	args, err := d.config.GetArgs(ddufPath, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get diffusers arguments: %w", err)
	}

	// Add served model name using the human-readable model reference
	if modelRef != "" {
		args = append(args, "--served-model-name", modelRef)
	}

	d.log.Infof("Diffusers args: %v", utils.SanitizeForLog(strings.Join(args, " ")))

	if d.pythonPath == "" {
		return fmt.Errorf("diffusers: python runtime not configured; did you forget to call Install")
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:      "Diffusers",
		Socket:           socket,
		BinaryPath:       d.pythonPath,
		SandboxPath:      "",
		SandboxConfig:    "",
		Args:             args,
		Logger:           d.log,
		ServerLogWriter:  d.serverLog.Writer(),
		ErrorTransformer: ExtractPythonError,
	})
}

// Status implements inference.Backend.Status.
func (d *diffusers) Status() string {
	return d.status
}

// GetDiskUsage implements inference.Backend.GetDiskUsage.
func (d *diffusers) GetDiskUsage() (int64, error) {
	// Return 0 if not installed
	if _, err := os.Stat(d.installDir); os.IsNotExist(err) {
		return 0, nil
	}

	var size int64
	err := filepath.Walk(d.installDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %w", err)
	}
	return size, nil
}
