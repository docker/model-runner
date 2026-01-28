package vllmmetal

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "vllm-metal"
	// defaultInstallDir is the directory where vllm-metal is installed.
	defaultInstallDir = ".model-runner/vllm-metal"
	// vllmMetalVersion is the version of vllm-metal to download.
	vllmMetalVersion = "v0.1.0"
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
	// version is the installed vllm-metal version.
	version string
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

	// Check for custom path first
	if v.customPythonPath != "" {
		v.pythonPath = v.customPythonPath
		return v.verifyInstallation(ctx)
	}

	// Check if already extracted
	pythonPath := filepath.Join(v.installDir, "bin", "python3")
	if _, err := os.Stat(pythonPath); err == nil {
		v.pythonPath = pythonPath
		return v.verifyInstallation(ctx)
	}

	// Download and extract tarball
	if err := v.downloadAndExtract(ctx, httpClient); err != nil {
		return fmt.Errorf("failed to install vllm-metal: %w", err)
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

// downloadAndExtract downloads the vllm-metal tarball from Docker Hub and extracts it.
func (v *vllmMetal) downloadAndExtract(ctx context.Context, httpClient *http.Client) error {
	pythonPath, pyVersion, err := v.findSystemPython(ctx)
	if err != nil {
		return err
	}

	v.log.Infof("Using system Python %s from %s", pyVersion, pythonPath)
	v.log.Infof("Downloading vllm-metal from Docker Hub...")

	imageRef := fmt.Sprintf("docker/model-runner:vllm-metal-%s", vllmMetalVersion)
	tarballReader, err := v.extractFromDockerImage(ctx, httpClient, imageRef)
	if err != nil {
		return fmt.Errorf("failed to download from Docker Hub: %w", err)
	}
	defer tarballReader.Close()

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
	if err := os.MkdirAll(sitePackagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create site-packages dir: %w", err)
	}

	if err := extractTarGzFromLayer(tarballReader, sitePackagesDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	v.log.Infof("vllm-metal installed successfully")
	return nil
}

// extractFromDockerImage pulls the vllm-metal image from Docker Hub and returns
// a reader for the layer contents (the /vllm-metal directory).
func (v *vllmMetal) extractFromDockerImage(ctx context.Context, httpClient *http.Client, imageRef string) (io.ReadCloser, error) {
	ref, err := reference.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageRef, err)
	}

	var transport http.RoundTripper = http.DefaultTransport
	if httpClient != nil && httpClient.Transport != nil {
		transport = httpClient.Transport
	}

	img, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithTransport(transport),
		remote.WithUserAgent("model-runner"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("image has no layers")
	}

	// Image has a single layer containing /vllm-metal/*
	layer := layers[0]
	compressed, err := layer.Compressed()
	if err != nil {
		return nil, fmt.Errorf("failed to get layer content: %w", err)
	}

	return compressed, nil
}

// extractTarGzFromLayer extracts files from a layer tarball, stripping the /vllm-metal prefix.
func extractTarGzFromLayer(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Strip the /vllm-metal/ prefix
		name := header.Name
		const prefix = "vllm-metal/"
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" || name == "." {
			continue
		}

		// Prevent path traversal
		cleanName := filepath.Clean(name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		target := filepath.Join(destDir, cleanName)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", target, err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			const maxFileSize = 1 << 30 // 1GB limit to prevent zip bombs
			if _, err := io.CopyN(f, tr, maxFileSize); err != nil && err != io.EOF {
				f.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			f.Close()
		case tar.TypeSymlink:
			linkTarget := filepath.Clean(header.Linkname)
			if strings.HasPrefix(linkTarget, "..") || filepath.IsAbs(linkTarget) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for symlink %s: %w", target, err)
			}
			_ = os.Symlink(header.Linkname, target)
		}
	}
	return nil
}

func (v *vllmMetal) verifyInstallation(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, v.pythonPath, "-c", "import vllm_metal; print(vllm_metal.__version__)")
	output, err := cmd.Output()
	if err != nil {
		v.status = "import failed"
		return fmt.Errorf("vllm_metal import failed: %w", err)
	}

	v.version = strings.TrimSpace(string(output))
	v.status = fmt.Sprintf("running vllm-metal version: %s", v.version)
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
