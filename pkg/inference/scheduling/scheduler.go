package scheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/middleware"
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
	// router is the HTTP request router.
	router *http.ServeMux
	// httpHandler is the HTTP request handler, which wraps router with
	// the server-level middleware.
	httpHandler http.Handler
	// tracker is the metrics tracker.
	tracker *metrics.Tracker
	// openAIRecorder is used to record OpenAI API inference requests and responses.
	openAIRecorder *metrics.OpenAIRecorder
	// lock is used to synchronize access to the scheduler's router.
	lock sync.RWMutex
}

// NewScheduler creates a new inference scheduler.
func NewScheduler(
	log logging.Logger,
	backends map[string]inference.Backend,
	defaultBackend inference.Backend,
	modelManager *models.Manager,
	httpClient *http.Client,
	allowedOrigins []string,
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
		router:         http.NewServeMux(),
		tracker:        tracker,
		openAIRecorder: openAIRecorder,
	}

	// Register routes.
	s.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range s.routeHandlers() {
		s.router.HandleFunc(route, handler)
	}

	s.RebuildRoutes(allowedOrigins)

	// Scheduler successfully initialized.
	return s
}

func (s *Scheduler) RebuildRoutes(allowedOrigins []string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	// Update handlers that depend on the allowed origins.
	s.httpHandler = middleware.CorsMiddleware(allowedOrigins, s.router)
}

func (s *Scheduler) routeHandlers() map[string]http.HandlerFunc {
	openAIRoutes := []string{
		"POST " + inference.InferencePrefix + "/{backend}/v1/chat/completions",
		"POST " + inference.InferencePrefix + "/{backend}/v1/completions",
		"POST " + inference.InferencePrefix + "/{backend}/v1/embeddings",
		"POST " + inference.InferencePrefix + "/v1/chat/completions",
		"POST " + inference.InferencePrefix + "/v1/completions",
		"POST " + inference.InferencePrefix + "/v1/embeddings",
	}
	m := make(map[string]http.HandlerFunc)
	for _, route := range openAIRoutes {
		m[route] = s.handleOpenAIInference
	}

	// Register /v1/models routes - these delegate to the model manager
	m["GET "+inference.InferencePrefix+"/{backend}/v1/models"] = s.handleModels
	m["GET "+inference.InferencePrefix+"/{backend}/v1/models/{name...}"] = s.handleModels
	m["GET "+inference.InferencePrefix+"/v1/models"] = s.handleModels
	m["GET "+inference.InferencePrefix+"/v1/models/{name...}"] = s.handleModels

	m["GET "+inference.InferencePrefix+"/status"] = s.GetBackendStatus
	m["GET "+inference.InferencePrefix+"/ps"] = s.GetRunningBackends
	m["GET "+inference.InferencePrefix+"/df"] = s.GetDiskUsage
	m["POST "+inference.InferencePrefix+"/unload"] = s.Unload
	m["POST "+inference.InferencePrefix+"/{backend}/_configure"] = s.Configure
	m["POST "+inference.InferencePrefix+"/_configure"] = s.Configure
	m["POST "+inference.InferencePrefix+"/{backend}/load"] = s.Load
	m["POST "+inference.InferencePrefix+"/load"] = s.Load
	m["GET "+inference.InferencePrefix+"/requests"] = s.openAIRecorder.GetRecordsHandler()
	return m
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

// handleOpenAIInference handles scheduling and responding to OpenAI inference
// requests, including:
// - POST <inference-prefix>/{backend}/v1/chat/completions
// - POST <inference-prefix>/{backend}/v1/completions
// - POST <inference-prefix>/{backend}/v1/embeddings
func (s *Scheduler) handleOpenAIInference(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	var backend inference.Backend
	if b := r.PathValue("backend"); b == "" {
		backend = s.defaultBackend
	} else {
		backend = s.backends[b]
	}
	if backend == nil {
		http.Error(w, ErrBackendNotFound.Error(), http.StatusNotFound)
		return
	}

	// Read the entire request body. We put some basic size constraints in place
	// to avoid DoS attacks. We do this early to avoid client write timeouts.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		}
		return
	}

	// Wait for the corresponding backend installation to complete or fail. We
	// don't allow any requests to be scheduled for a backend until it has
	// completed installation.
	if err := s.installer.wait(r.Context(), backend.Name()); err != nil {
		if errors.Is(err, ErrBackendNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, errInstallerNotStarted) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else if errors.Is(err, context.Canceled) {
			// This could be due to the client aborting the request (in which
			// case this response will be ignored) or the inference service
			// shutting down (since that will also cancel the request context).
			// Either way, provide a response, even if it's ignored.
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, fmt.Errorf("backend installation failed: %w", err).Error(), http.StatusServiceUnavailable)
		}
		return
	}

	// Determine the backend operation mode.
	backendMode, ok := backendModeForRequest(r.URL.Path)
	if !ok {
		http.Error(w, "unknown request path", http.StatusInternalServerError)
		return
	}

	// Decode the model specification portion of the request body.
	var request OpenAIInferenceRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if request.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	// Check if the shared model manager has the requested model available.
	if !backend.UsesExternalModelManagement() {
		model, err := s.modelManager.GetModel(request.Model)
		if err != nil {
			if errors.Is(err, distribution.ErrModelNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, "model unavailable", http.StatusInternalServerError)
			}
			return
		}
		// Non-blocking call to track the model usage.
		s.tracker.TrackModel(model, r.UserAgent(), "inference/"+backendMode.String())
	}

	modelID := s.modelManager.ResolveModelID(request.Model)

	// Request a runner to execute the request and defer its release.
	runner, err := s.loader.load(r.Context(), backend.Name(), modelID, request.Model, backendMode)
	if err != nil {
		http.Error(w, fmt.Errorf("unable to load runner: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	defer s.loader.release(runner)

	// Record the request in the OpenAI recorder.
	recordID := s.openAIRecorder.RecordRequest(request.Model, r, body)
	w = s.openAIRecorder.NewResponseRecorder(w)
	defer func() {
		// Record the response in the OpenAI recorder.
		s.openAIRecorder.RecordResponse(recordID, request.Model, w)
	}()

	// Create a request with the body replaced for forwarding upstream.
	upstreamRequest := r.Clone(r.Context())
	upstreamRequest.Body = io.NopCloser(bytes.NewReader(body))

	// Perform the request.
	runner.ServeHTTP(w, upstreamRequest)
}

func (s *Scheduler) GetBackendStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]string)
	for backendName, backend := range s.backends {
		status[backendName] = backend.Status()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Scheduler) ResetInstaller(httpClient *http.Client) {
	s.installer = newInstaller(s.log, s.backends, httpClient)
}

// GetRunningBackends returns information about all running backends
func (s *Scheduler) GetRunningBackends(w http.ResponseWriter, r *http.Request) {
	runningBackends := s.getLoaderStatus(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(runningBackends); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
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
			}

			if s.loader.references[runnerInfo.slot] == 0 {
				status.LastUsed = s.loader.timestamps[runnerInfo.slot]
			}

			result = append(result, status)
		}
	}

	return result
}

func (s *Scheduler) GetDiskUsage(w http.ResponseWriter, _ *http.Request) {
	modelsDiskUsage, err, httpCode := s.modelManager.GetDiskUsage()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get models disk usage: %v", err), httpCode)
		return
	}

	// TODO: Get disk usage for each backend once the backends are implemented.
	defaultBackendDiskUsage, err := s.defaultBackend.GetDiskUsage()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get disk usage for %s: %v", s.defaultBackend.Name(), err), http.StatusInternalServerError)
		return
	}

	diskUsage := DiskUsage{modelsDiskUsage, defaultBackendDiskUsage}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(diskUsage); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// Unload unloads the specified runners (backend, model) from the backend.
// Currently, this doesn't work for runners that are handling an OpenAI request.
func (s *Scheduler) Unload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		}
		return
	}

	var unloadRequest UnloadRequest
	if err := json.Unmarshal(body, &unloadRequest); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	unloadedRunners := UnloadResponse{s.loader.Unload(r.Context(), unloadRequest)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(unloadedRunners); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *Scheduler) Configure(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	var backend inference.Backend
	if b := r.PathValue("backend"); b == "" {
		backend = s.defaultBackend
	} else {
		backend = s.backends[b]
	}
	if backend == nil {
		http.Error(w, ErrBackendNotFound.Error(), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		}
		return
	}

	configureRequest := ConfigureRequest{
		ContextSize: -1,
	}
	if err := json.Unmarshal(body, &configureRequest); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var runtimeFlags []string
	if len(configureRequest.RuntimeFlags) > 0 {
		runtimeFlags = configureRequest.RuntimeFlags
	} else {
		rawFlags, err := shellwords.Parse(configureRequest.RawRuntimeFlags)
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runtimeFlags = rawFlags
	}

	var runnerConfig inference.BackendConfiguration
	runnerConfig.ContextSize = configureRequest.ContextSize
	runnerConfig.RuntimeFlags = runtimeFlags

	mode := inference.BackendModeCompletion
	if slices.Contains(runnerConfig.RuntimeFlags, "--embeddings") {
		mode = inference.BackendModeEmbedding
	}

	if model, err := s.modelManager.GetModel(configureRequest.Model); err == nil {
		// Configure is called by compose for each model.
		s.tracker.TrackModel(model, r.UserAgent(), "configure/"+mode.String())
	}
	modelID := s.modelManager.ResolveModelID(configureRequest.Model)
	if err := s.loader.setRunnerConfig(r.Context(), backend.Name(), modelID, mode, runnerConfig); err != nil {
		s.log.Warnf("Failed to configure %s runner for %s (%s): %s", backend.Name(), configureRequest.Model, modelID, err)
		if errors.Is(err, errRunnerAlreadyActive) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Load handles loading a model into memory without performing inference.
// This is useful for warming up models in detached mode.
func (s *Scheduler) Load(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	var backend inference.Backend
	if b := r.PathValue("backend"); b == "" {
		backend = s.defaultBackend
	} else {
		backend = s.backends[b]
	}
	if backend == nil {
		http.Error(w, ErrBackendNotFound.Error(), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		}
		return
	}

	var loadRequest LoadRequest
	if err := json.Unmarshal(body, &loadRequest); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if loadRequest.Model == "" {
		http.Error(w, "model name is required", http.StatusBadRequest)
		return
	}

	// Wait for the backend installation to complete
	if err := s.installer.wait(r.Context(), backend.Name()); err != nil {
		if errors.Is(err, ErrBackendNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, errInstallerNotStarted) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else if errors.Is(err, context.Canceled) {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, fmt.Errorf("backend installation failed: %w", err).Error(), http.StatusServiceUnavailable)
		}
		return
	}

	// Resolve the model ID
	modelID := s.modelManager.ResolveModelID(loadRequest.Model)

	// Load the model using the loader (default to completion mode)
	mode := inference.BackendModeCompletion
	runner, err := s.loader.load(r.Context(), backend.Name(), modelID, loadRequest.Model, mode)
	if err != nil {
		s.log.Warnf("Failed to load model %s (%s): %v", loadRequest.Model, modelID, err)
		if errors.Is(err, errModelTooBig) {
			http.Error(w, "model too big for available memory", http.StatusInsufficientStorage)
		} else if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
		} else {
			http.Error(w, fmt.Sprintf("failed to load model: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Release the runner immediately since we're just loading it, not using it
	defer s.loader.release(runner)

	// Return success response
	response := LoadResponse{
		Status:  "loaded",
		Message: fmt.Sprintf("Model %s loaded successfully", loadRequest.Model),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Warnf("Failed to encode load response: %v", err)
	}
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
		// Find the runner slot for this backend/model combination
		key := runnerKey{
			backend: backend.BackendName,
			modelID: backend.ModelName,
			mode:    parseBackendMode(backend.Mode),
		}

		if runnerInfo, exists := s.loader.runners[key]; exists {
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
		if backend.BackendName == "llama.cpp" {
			// Find the runner slot for this backend/model combination
			key := runnerKey{
				backend: backend.BackendName,
				modelID: backend.ModelName,
				mode:    parseBackendMode(backend.Mode),
			}

			if runnerInfo, exists := s.loader.runners[key]; exists {
				// Use the RunnerSocketPath function to get the socket path
				return RunnerSocketPath(runnerInfo.slot)
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

// handleModels handles GET /engines/{backend}/v1/models* requests
// by delegating to the model manager
func (s *Scheduler) handleModels(w http.ResponseWriter, r *http.Request) {
	s.modelManager.ServeHTTP(w, r)
}

// ServeHTTP implements net/http.Handler.ServeHTTP.
func (s *Scheduler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	s.httpHandler.ServeHTTP(w, r)
}
