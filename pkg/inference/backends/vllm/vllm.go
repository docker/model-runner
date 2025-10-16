package vllm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/sandbox"
	"github.com/docker/model-runner/pkg/tailbuffer"
)

const (
	// Name is the backend name.
	Name    = "vllm"
	vllmDir = "/opt/vllm-env/bin/"
)

// vLLM is the vLLM-based backend implementation.
type vLLM struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the vLLM server process.
	serverLog logging.Logger
	// config is the configuration for the vLLM backend.
	config *Config
	// status is the state in which the vLLM backend is in.
	status string
}

// New creates a new vLLM-based backend.
func New(log logging.Logger, modelManager *models.Manager, serverLog logging.Logger, conf *Config) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultVLLMConfig()
	}

	return &vLLM{
		log:          log,
		modelManager: modelManager,
		serverLog:    serverLog,
		config:       conf,
		status:       "not installed",
	}, nil
}

// Name implements inference.Backend.Name.
func (v *vLLM) Name() string {
	return Name
}

func (v *vLLM) UsesExternalModelManagement() bool {
	return false
}

func (v *vLLM) Install(_ context.Context, _ *http.Client) error {
	if !platform.SupportsVLLM() {
		return errors.New("not implemented")
	}

	vllmBinaryPath := v.binaryPath()
	if _, err := os.Stat(vllmBinaryPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("vLLM binary not found at %s", vllmBinaryPath)
		}
		return fmt.Errorf("failed to check vLLM binary: %w", err)
	}

	// TODO: Find a way to get vllm's version. Running `vllm --version` is too slow.
	v.status = "running"

	return nil
}

func (v *vLLM) Run(ctx context.Context, socket, model string, modelRef string, _ inference.BackendMode, _ *inference.BackendConfiguration) error {
	if !platform.SupportsVLLM() {
		v.log.Warn("vLLM backend is not yet supported")
		return errors.New("not implemented")
	}

	bundle, err := v.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	if err := os.RemoveAll(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
		v.log.Warnf("failed to remove socket file %s: %v\n", socket, err)
		v.log.Warnln("vLLM may not be able to start")
	}

	args := []string{
		"serve",
		filepath.Dir(bundle.SafetensorsPath()),
		"--uds", socket,
		"--served-model-name", modelRef,
	}
	// TODO: Add inference.BackendConfiguration.

	v.log.Infof("vLLM args: %v", args)
	tailBuf := tailbuffer.NewTailBuffer(1024)
	serverLogStream := v.serverLog.Writer()
	out := io.MultiWriter(serverLogStream, tailBuf)
	vllmSandbox, err := sandbox.Create(
		ctx,
		"",
		func(command *exec.Cmd) {
			command.Cancel = func() error {
				if runtime.GOOS == "windows" {
					return command.Process.Kill()
				}
				return command.Process.Signal(os.Interrupt)
			}
			command.Stdout = serverLogStream
			command.Stderr = out
		},
		vllmDir,
		v.binaryPath(),
		args...,
	)
	if err != nil {
		return fmt.Errorf("unable to start vLLM: %w", err)
	}
	defer vllmSandbox.Close()

	vllmErrors := make(chan error, 1)
	go func() {
		vllmErr := vllmSandbox.Command().Wait()
		serverLogStream.Close()

		errOutput := new(strings.Builder)
		if _, err := io.Copy(errOutput, tailBuf); err != nil {
			v.log.Warnf("failed to read server output tail: %v", err)
		}

		if len(errOutput.String()) != 0 {
			vllmErr = fmt.Errorf("vLLM exit status: %w\nwith output: %s", vllmErr, errOutput.String())
		} else {
			vllmErr = fmt.Errorf("vLLM exit status: %w", vllmErr)
		}

		vllmErrors <- vllmErr
		close(vllmErrors)
		if err := os.Remove(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
			v.log.Warnf("failed to remove socket file %s on exit: %v\n", socket, err)
		}
	}()
	defer func() {
		<-vllmErrors
	}()

	select {
	case <-ctx.Done():
		return nil
	case vllmErr := <-vllmErrors:
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return fmt.Errorf("vLLM terminated unexpectedly: %w", vllmErr)
	}
}

func (v *vLLM) Status() string {
	return v.status
}

func (v *vLLM) GetDiskUsage() (int64, error) {
	size, err := diskusage.Size(vllmDir)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %v", err)
	}
	return size, nil
}

func (v *vLLM) GetRequiredMemoryForModel(_ context.Context, _ string, _ *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	if !platform.SupportsVLLM() {
		return inference.RequiredMemory{}, errors.New("not implemented")
	}

	return inference.RequiredMemory{
		RAM:  1,
		VRAM: 1,
	}, nil
}

func (v *vLLM) binaryPath() string {
	return filepath.Join(vllmDir, "vllm")
}
