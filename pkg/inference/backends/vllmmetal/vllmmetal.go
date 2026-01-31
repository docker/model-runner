package vllmmetal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/internal/dockerhub"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name              = "vllm-metal"
	defaultInstallDir = ".docker/model-runner/vllm-metal"
	// vllmMetalVersion is the vllm-metal release tag to download from Docker Hub.
	vllmMetalVersion = "v0.1.0-20260126-121650"
)

var (
	// ErrPlatformNotSupported indicates the platform is not supported.
	ErrPlatformNotSupported = errors.New("vllm-metal is only available on macOS ARM64")
)

// vllmMetal is the vllm-metal backend implementation using MLX for Apple Silicon.
type vllmMetal struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the vllm-metal server process.
	serverLog logging.Logger
	// pythonPath is the path to the python3 binary in the venv.
	pythonPath string
	// customPythonPath is an optional custom path to a python3 binary.
	customPythonPath string
	// installDir is the directory where vllm-metal is installed.
	installDir string
	// status is the state in which the backend is in.
	status string
}

// New creates a new vllm-metal backend.
// customPythonPath is an optional path to a custom python3 binary; if empty, the default installation is used.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, customPythonPath string) (inference.Backend, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	installDir := filepath.Join(homeDir, defaultInstallDir)

	return &vllmMetal{
		log:              log,
		modelManager:     modelManager,
		serverLog:        serverLog,
		customPythonPath: customPythonPath,
		installDir:       installDir,
		status:           "not installed",
	}, nil
}

// Name implements inference.Backend.Name.
func (v *vllmMetal) Name() string {
	return Name
}

// UsesExternalModelManagement implements inference.Backend.UsesExternalModelManagement.
func (v *vllmMetal) UsesExternalModelManagement() bool {
	return false
}

// UsesTCP implements inference.Backend.UsesTCP.
// vllm-metal uses TCP for communication as it runs a FastAPI server.
func (v *vllmMetal) UsesTCP() bool {
	return true
}

// Install implements inference.Backend.Install.
func (v *vllmMetal) Install(ctx context.Context, httpClient *http.Client) error {
	if !platform.SupportsVLLMMetal() {
		return ErrPlatformNotSupported
	}

	if v.customPythonPath != "" {
		v.pythonPath = v.customPythonPath
		return v.verifyInstallation(ctx)
	}

	pythonPath := filepath.Join(v.installDir, "bin", "python3")
	versionFile := filepath.Join(v.installDir, ".vllm-metal-version")

	// Check if already installed with correct version
	if _, err := os.Stat(pythonPath); err == nil {
		if installedVersion, err := os.ReadFile(versionFile); err == nil {
			installed := strings.TrimSpace(string(installedVersion))
			if installed == vllmMetalVersion || installed == "dev" {
				v.pythonPath = pythonPath
				return v.verifyInstallation(ctx)
			}
			v.log.Infof("vllm-metal version mismatch: installed %s, want %s", installed, vllmMetalVersion)
		}
	}

	v.status = "installing"
	if err := v.downloadAndExtract(ctx, httpClient); err != nil {
		return fmt.Errorf("failed to install vllm-metal: %w", err)
	}

	// Save version file
	if err := os.WriteFile(versionFile, []byte(vllmMetalVersion), 0644); err != nil {
		v.log.Warnf("failed to write version file: %v", err)
	}

	v.pythonPath = pythonPath
	return v.verifyInstallation(ctx)
}

// findSystemPython finds Python 3.12 interpreter on the system.
// Python 3.12 is required because the vllm-metal wheel is built for cp312.
func (v *vllmMetal) findSystemPython(ctx context.Context) (string, string, error) {
	// Try python3.12 first
	for _, pyCmd := range []string{"python3.12", "python3"} {
		pythonPath, err := exec.LookPath(pyCmd)
		if err != nil {
			continue
		}

		// Verify version is exactly 3.12
		out, err := exec.CommandContext(ctx, pythonPath, "--version").Output()
		if err != nil {
			continue
		}

		versionStr := strings.TrimPrefix(strings.TrimSpace(string(out)), "Python ")
		parts := strings.Split(versionStr, ".")
		if len(parts) < 2 {
			continue
		}

		major, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		// Must be exactly Python 3.12 for wheel compatibility
		if major == 3 && minor == 12 {
			return pythonPath, "3.12", nil
		}
	}

	return "", "", fmt.Errorf("python 3.12 required (vllm-metal wheel is built for cp312); install with: brew install python@3.12")
}

// downloadAndExtract downloads the vllm-metal image from Docker Hub and extracts it.
func (v *vllmMetal) downloadAndExtract(ctx context.Context, _ *http.Client) error {
	pythonPath, pyVersion, err := v.findSystemPython(ctx)
	if err != nil {
		return err
	}

	v.log.Infof("Using system Python %s from %s", pyVersion, pythonPath)
	v.log.Infof("Downloading vllm-metal %s from Docker Hub...", vllmMetalVersion)

	// Create temp directory for download
	downloadDir, err := os.MkdirTemp("", "vllm-metal-install")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	// Pull the image
	image := fmt.Sprintf("registry-1.docker.io/docker/model-runner:vllm-metal-%s", vllmMetalVersion)
	if err := dockerhub.PullPlatform(ctx, image, filepath.Join(downloadDir, "image.tar"), runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Extract the image
	extractDir := filepath.Join(downloadDir, "extracted")
	if err := dockerhub.Extract(filepath.Join(downloadDir, "image.tar"), runtime.GOARCH, runtime.GOOS, extractDir); err != nil {
		return fmt.Errorf("failed to extract image: %w", err)
	}

	v.log.Infof("Creating Python venv at %s...", v.installDir)
	if err := os.MkdirAll(filepath.Dir(v.installDir), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	// Remove existing install dir if it exists (incomplete installation)
	if err := os.RemoveAll(v.installDir); err != nil {
		return fmt.Errorf("failed to remove existing install dir: %w", err)
	}

	venvCmd := exec.CommandContext(ctx, pythonPath, "-m", "venv", v.installDir)
	if out, err := venvCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create venv: %w\nOutput: %s", err, string(out))
	}

	v.log.Infof("Extracting packages...")
	sitePackagesDir := filepath.Join(v.installDir, "lib", fmt.Sprintf("python%s", pyVersion), "site-packages")

	// Copy extracted files to site-packages (the image contains /vllm-metal/*)
	vllmMetalDir := filepath.Join(extractDir, "vllm-metal")
	if err := copyDir(vllmMetalDir, sitePackagesDir); err != nil {
		return fmt.Errorf("failed to copy to site-packages: %w", err)
	}

	v.log.Infof("vllm-metal %s installed successfully", vllmMetalVersion)
	return nil
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, dstPath)
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = dstFile.ReadFrom(srcFile)
		return err
	})
}

func (v *vllmMetal) verifyInstallation(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, v.pythonPath, "-c", "import vllm_metal")
	if err := cmd.Run(); err != nil {
		v.status = "import failed"
		return fmt.Errorf("vllm_metal import failed: %w", err)
	}

	versionFile := filepath.Join(v.installDir, ".vllm-metal-version")
	versionBytes, err := os.ReadFile(versionFile)
	if err != nil {
		v.status = "running vllm-metal"
		return nil
	}
	v.status = fmt.Sprintf("running vllm-metal %s", strings.TrimSpace(string(versionBytes)))
	return nil
}

// Run implements inference.Backend.Run.
func (v *vllmMetal) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	if !platform.SupportsVLLMMetal() {
		return ErrPlatformNotSupported
	}

	bundle, err := v.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	args, err := v.buildArgs(bundle, socket, mode, config)
	if err != nil {
		return fmt.Errorf("failed to build vllm-metal arguments: %w", err)
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "vllm-metal",
		Socket:          socket,
		BinaryPath:      v.pythonPath,
		SandboxPath:     "",
		SandboxConfig:   "",
		Args:            args,
		Logger:          v.log,
		ServerLogWriter: v.serverLog.Writer(),
	})
}

// buildArgs builds the command line arguments for vllm-metal server.
func (v *vllmMetal) buildArgs(bundle interface{ SafetensorsPath() string }, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error) {
	// Parse host:port from socket (vllm-metal uses TCP)
	host, port, err := net.SplitHostPort(socket)
	if err != nil {
		return nil, fmt.Errorf("invalid socket format (expected host:port): %w", err)
	}

	// Get model path from safetensors
	safetensorsPath := bundle.SafetensorsPath()
	if safetensorsPath == "" {
		return nil, fmt.Errorf("safetensors path required by vllm-metal backend")
	}
	modelPath := filepath.Dir(safetensorsPath)

	args := []string{
		"-m", "vllm_metal.server",
		"--model", modelPath,
		"--host", host,
		"--port", port,
	}

	// Add mode-specific arguments
	switch mode {
	case inference.BackendModeCompletion:
		// Default mode, no additional args needed
	case inference.BackendModeEmbedding:
		// vllm-metal may handle embedding models automatically
	case inference.BackendModeReranking:
		return nil, fmt.Errorf("reranking mode not supported by vllm-metal backend")
	case inference.BackendModeImageGeneration:
		return nil, fmt.Errorf("image generation mode not supported by vllm-metal backend")
	}

	// Add context size if specified
	if config != nil && config.ContextSize != nil {
		args = append(args, "--max-model-len", strconv.Itoa(int(*config.ContextSize)))
	}

	// Add runtime flags if specified
	if config != nil && len(config.RuntimeFlags) > 0 {
		args = append(args, config.RuntimeFlags...)
	}

	return args, nil
}

// Status implements inference.Backend.Status.
func (v *vllmMetal) Status() string {
	return v.status
}

// GetDiskUsage implements inference.Backend.GetDiskUsage.
func (v *vllmMetal) GetDiskUsage() (int64, error) {
	// Return 0 if not installed
	if _, err := os.Stat(v.installDir); os.IsNotExist(err) {
		return 0, nil
	}

	var size int64
	err := filepath.Walk(v.installDir, func(_ string, info os.FileInfo, err error) error {
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
