package scheduling

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/mattn/go-shellwords"
	"golang.org/x/sync/errgroup"
)

// Scheduler is used to coordinate inference scheduling across multiple backends
// and models.
type Scheduler struct {
	// log is the associated logger.
	log logging.Logger
	// backends are the supported inference backends.
	backends map[string]inference.Backend
	// defaultBackend is the default inference backend. It may be nil.
	defaultBackend inference.Backend
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// installer is the backend installer.
	installer *installer
	// loader is the backend loader.
	loader *loader
	// tracker is the metrics tracker.
	tracker *metrics.Tracker
	// openAIRecorder is used to record OpenAI API inference requests and responses.
	openAIRecorder *metrics.OpenAIRecorder
}

// NewScheduler creates a new inference scheduler.
func NewScheduler(
	log logging.Logger,
	backends map[string]inference.Backend,
	defaultBackend inference.Backend,
	modelManager *models.Manager,
	httpClient *http.Client,
	tracker *metrics.Tracker,
	sysMemInfo memory.SystemMemoryInfo,
) *Scheduler {
	openAIRecorder := metrics.NewOpenAIRecorder(log.WithField("component", "openai-recorder"), modelManager)

	// Create the scheduler.
	s := &Scheduler{
		log:            log,
		backends:       backends,
		defaultBackend: defaultBackend,
		modelManager:   modelManager,
		installer:      newInstaller(log, backends, httpClient),
		loader:         newLoader(log, backends, modelManager, openAIRecorder, sysMemInfo),
		tracker:        tracker,
		openAIRecorder: openAIRecorder,
	}

	// Scheduler successfully initialized.
	return s
}

// Run is the scheduler's main run loop. By the time it returns, all inference
// backends will have been unloaded from memory.
func (s *Scheduler) Run(ctx context.Context) error {
	// Create an error group to track worker Goroutines.
	workers, workerCtx := errgroup.WithContext(ctx)

	// Start the installer.
	workers.Go(func() error {
		s.installer.run(workerCtx)
		return nil
	})

	// Start the loader.
	workers.Go(func() error {
		s.loader.run(workerCtx)
		return nil
	})

	// Wait for all workers to exit.
	return workers.Wait()
}

// selectBackendForModel selects the appropriate backend for a model based on its format.
// If the model is in safetensors format, it will prefer vLLM if available.
func (s *Scheduler) selectBackendForModel(model types.Model, backend inference.Backend, modelRef string) inference.Backend {
	config, err := model.Config()
	if err != nil {
		s.log.Warnln("failed to fetch model config:", err)
		return backend
	}

	if config.Format == types.FormatSafetensors {
		if vllmBackend, ok := s.backends[vllm.Name]; ok && vllmBackend != nil {
			return vllmBackend
		}
		s.log.Warnf("Model %s is in safetensors format but vLLM backend is not available. "+
			"Backend %s may not support this format and could fail at runtime.",
			utils.SanitizeForLog(modelRef), backend.Name())
	}

	return backend
}

// ResetInstaller resets the backend installer with a new HTTP client.
func (s *Scheduler) ResetInstaller(httpClient *http.Client) {
	s.installer = newInstaller(s.log, s.backends, httpClient)
}

// GetRunningBackendsInfo returns information about all running backends as a slice
func (s *Scheduler) GetRunningBackendsInfo(ctx context.Context) []BackendStatus {
	return s.getLoaderStatus(ctx)
}

// getLoaderStatus returns information about all running backends managed by the loader
func (s *Scheduler) getLoaderStatus(ctx context.Context) []BackendStatus {
	if !s.loader.lock(ctx) {
		return []BackendStatus{}
	}
	defer s.loader.unlock()

	result := make([]BackendStatus, 0, len(s.loader.runners))

	for key, runnerInfo := range s.loader.runners {
		if s.loader.slots[runnerInfo.slot] != nil {
			status := BackendStatus{
				BackendName: key.backend,
				ModelName:   runnerInfo.modelRef,
				Mode:        key.mode.String(),
				LastUsed:    time.Time{},
				InUse:       s.loader.references[runnerInfo.slot] > 0,
			}

			if s.loader.references[runnerInfo.slot] == 0 {
				status.LastUsed = s.loader.timestamps[runnerInfo.slot]
			}

			result = append(result, status)
		}
	}

	return result
}

// GetAllActiveRunners returns information about all active runners
func (s *Scheduler) GetAllActiveRunners() []metrics.ActiveRunner {
	runningBackends := s.getLoaderStatus(context.Background())
	var activeRunners []metrics.ActiveRunner

	if !s.loader.lock(context.Background()) {
		return activeRunners
	}
	defer s.loader.unlock()

	for _, backend := range runningBackends {
		mode := parseBackendMode(backend.Mode)
		// Find the runner slot for this backend/model combination
		// We iterate through all runners since we don't know the draftModelID
		for key, runnerInfo := range s.loader.runners {
			if key.backend == backend.BackendName && key.modelID == backend.ModelName && key.mode == mode {
				socket, err := RunnerSocketPath(runnerInfo.slot)
				if err != nil {
					s.log.Warnf("Failed to get socket path for runner %s/%s (%s): %v", backend.BackendName, backend.ModelName, key.modelID, err)
					continue
				}

				activeRunners = append(activeRunners, metrics.ActiveRunner{
					BackendName: backend.BackendName,
					ModelName:   backend.ModelName,
					Mode:        backend.Mode,
					Socket:      socket,
				})
				break // Found the runner, no need to continue iterating
			}
		}
	}

	return activeRunners
}

// GetLlamaCppSocket returns the Unix socket path for an active llama.cpp runner
func (s *Scheduler) GetLlamaCppSocket() (string, error) {
	runningBackends := s.getLoaderStatus(context.Background())

	if !s.loader.lock(context.Background()) {
		return "", errors.New("failed to acquire loader lock")
	}
	defer s.loader.unlock()

	// Look for an active llama.cpp backend
	for _, backend := range runningBackends {
		if backend.BackendName == llamacpp.Name {
			mode := parseBackendMode(backend.Mode)
			// Find the runner slot for this backend/model combination
			// We iterate through all runners since we don't know the draftModelID
			for key, runnerInfo := range s.loader.runners {
				if key.backend == backend.BackendName && key.modelID == backend.ModelName && key.mode == mode {
					// Use the RunnerSocketPath function to get the socket path
					return RunnerSocketPath(runnerInfo.slot)
				}
			}
		}
	}

	return "", errors.New("no active llama.cpp backend found")
}

// parseBackendMode converts a string mode to BackendMode
func parseBackendMode(mode string) inference.BackendMode {
	switch mode {
	case "completion":
		return inference.BackendModeCompletion
	case "embedding":
		return inference.BackendModeEmbedding
	default:
		return inference.BackendModeCompletion
	}
}

// ConfigureRunner configures a runner for a specific model and backend.
// It handles all the business logic of configuration including parsing flags,
// determining mode, selecting backend, and setting runner configuration.
func (s *Scheduler) ConfigureRunner(ctx context.Context, backend inference.Backend, req ConfigureRequest, userAgent string) (inference.Backend, error) {
	if backend == nil {
		backend = s.defaultBackend
	}

	// Parse runtime flags from either array or raw string
	var runtimeFlags []string
	if len(req.RuntimeFlags) > 0 {
		runtimeFlags = req.RuntimeFlags
	} else if req.RawRuntimeFlags != "" {
		var err error
		runtimeFlags, err = shellwords.Parse(req.RawRuntimeFlags)
		if err != nil {
			return nil, fmt.Errorf("invalid runtime flags: %w", err)
		}
	}

	// Build runner configuration
	var runnerConfig inference.BackendConfiguration
	runnerConfig.ContextSize = req.ContextSize
	runnerConfig.RuntimeFlags = runtimeFlags
	runnerConfig.Speculative = req.Speculative

	// Determine mode from flags
	mode := inference.BackendModeCompletion
	if slices.Contains(runnerConfig.RuntimeFlags, "--embeddings") {
		mode = inference.BackendModeEmbedding
	}

	// Get model, track usage, and select appropriate backend
	if model, err := s.modelManager.GetLocal(req.Model); err == nil {
		// Configure is called by compose for each model
		s.tracker.TrackModel(model, userAgent, "configure/"+mode.String())

		// Automatically identify models for vLLM
		backend = s.selectBackendForModel(model, backend, req.Model)
	}

	// Resolve model ID
	modelID := s.modelManager.ResolveID(req.Model)

	// Set the runner configuration
	if err := s.loader.setRunnerConfig(ctx, backend.Name(), modelID, mode, runnerConfig); err != nil {
		s.log.Warnf("Failed to configure %s runner for %s (%s): %s", backend.Name(), utils.SanitizeForLog(req.Model, -1), modelID, err)
		return nil, err
	}

	return backend, nil
}
