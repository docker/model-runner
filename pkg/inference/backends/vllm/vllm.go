package vllm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name    = "vllm"
	vllmDir = "/opt/vllm-env/bin"
)

var ErrorNotFound = errors.New("vLLM binary not found")

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
			v.status = ErrorNotFound.Error()
			return ErrorNotFound
		}
		return fmt.Errorf("failed to check vLLM binary: %w", err)
	}

	// Read vLLM version from file (created in Dockerfile via `print(vllm.__version__)`).
	versionPath := filepath.Join(filepath.Dir(vllmDir), "version")
	versionBytes, err := os.ReadFile(versionPath)
	if err != nil {
		v.log.Warnf("could not get vllm version: %v", err)
		v.status = "running vllm version: unknown"
	} else {
		v.status = fmt.Sprintf("running vllm version: %s", strings.TrimSpace(string(versionBytes)))
	}

	return nil
}

func (v *vLLM) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, backendConfig *inference.BackendConfiguration) error {
	if !platform.SupportsVLLM() {
		v.log.Warn("vLLM backend is not yet supported")
		return errors.New("not implemented")
	}

	bundle, err := v.modelManager.GetBundle(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	var draftBundle types.ModelBundle
	if backendConfig != nil && backendConfig.Speculative != nil && backendConfig.Speculative.DraftModel != "" {
		draftBundle, err = v.modelManager.GetBundle(backendConfig.Speculative.DraftModel)
		if err != nil {
			return fmt.Errorf("failed to get draft model: %w", err)
		}
	}

	args, err := v.config.GetArgs(bundle, socket, mode, backendConfig)
	if err != nil {
		return fmt.Errorf("failed to get vLLM arguments: %w", err)
	}

	if draftBundle != nil && backendConfig != nil && backendConfig.Speculative != nil {
		draftPath := draftBundle.SafetensorsPath()
		if draftPath != "" {
			// vLLM uses --speculative_config with a JSON string
			// Dynamically construct JSON, omitting num_speculative_tokens if not set
			speculativeConfigMap := map[string]interface{}{
				"model": filepath.Dir(draftPath),
			}
			if backendConfig.Speculative.NumTokens > 0 {
				speculativeConfigMap["num_speculative_tokens"] = backendConfig.Speculative.NumTokens
			}
			speculativeConfigBytes, err := json.Marshal(speculativeConfigMap)
			if err != nil {
				return fmt.Errorf("failed to marshal speculative config for vLLM: %w", err)
			}
			speculativeConfig := string(speculativeConfigBytes)
			args = append(args, "--speculative_config", speculativeConfig)
			args = append(args, "--use-v2-block-manager")
		}
	}

	args = append(args, "--served-model-name", model, modelRef)

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "vLLM",
		Socket:          socket,
		BinaryPath:      v.binaryPath(),
		SandboxPath:     vllmDir,
		SandboxConfig:   "",
		Args:            args,
		Logger:          v.log,
		ServerLogWriter: v.serverLog.Writer(),
	})
}

func (v *vLLM) Status() string {
	return v.status
}

func (v *vLLM) GetDiskUsage() (int64, error) {
	size, err := diskusage.Size(vllmDir)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %w", err)
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
