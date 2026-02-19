package routing

import (
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/mlx"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

// BackendsConfig configures which inference backends to create and how.
type BackendsConfig struct {
	// Log is the main logger passed to each backend.
	Log logging.Logger

	// ServerLogFactory creates the server-process logger for a backend.
	// If nil, Log is used directly as the server logger.
	ServerLogFactory func(backendName string) logging.Logger

	// LlamaCpp settings (always included).
	LlamaCppVendoredPath string
	LlamaCppUpdatedPath  string
	LlamaCppConfig       config.BackendConfig

	// Optional backends and their custom server paths.
	IncludeMLX bool
	MLXPath    string

	IncludeVLLM   bool
	VLLMPath      string
	VLLMMetalPath string
}

// DefaultBackendDefs returns BackendDef entries for the configured backends.
// It always includes llamacpp; MLX and vLLM are included based on the
// boolean flags.
func DefaultBackendDefs(cfg BackendsConfig) []BackendDef {
	sl := func(name string) logging.Logger {
		if cfg.ServerLogFactory != nil {
			return cfg.ServerLogFactory(name)
		}
		return cfg.Log
	}

	defs := []BackendDef{
		{Name: llamacpp.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
			return llamacpp.New(cfg.Log, mm, sl(llamacpp.Name), cfg.LlamaCppVendoredPath, cfg.LlamaCppUpdatedPath, cfg.LlamaCppConfig)
		}},
	}

	if cfg.IncludeMLX {
		defs = append(defs, BackendDef{Name: mlx.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
			return mlx.New(cfg.Log, mm, sl(mlx.Name), nil, cfg.MLXPath)
		}})
	}

	if cfg.IncludeVLLM {
		defs = append(defs, BackendDef{
			Name:     vllm.Name,
			Deferred: vllm.NeedsDeferredInstall(),
			Init: func(mm *models.Manager) (inference.Backend, error) {
				return vllm.New(cfg.Log, mm, sl(vllm.Name), vllm.Options{
					LinuxBinaryPath: cfg.VLLMPath,
					MetalPythonPath: cfg.VLLMMetalPath,
				})
			},
		})
	}

	return defs
}
