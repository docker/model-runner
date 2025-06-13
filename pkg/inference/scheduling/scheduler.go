package scheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
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
	// router is the HTTP request router.
	router *http.ServeMux
	// tracker is the metrics tracker.
	tracker *metrics.Tracker
	// lock is used to synchronize access to the scheduler's router.
	lock sync.Mutex
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
) *Scheduler {
	// Create the scheduler.
	s := &Scheduler{
		log:            log,
		backends:       backends,
		defaultBackend: defaultBackend,
		modelManager:   modelManager,
		installer:      newInstaller(log, backends, httpClient),
		loader:         newLoader(log, backends, modelManager),
		router:         http.NewServeMux(),
		tracker:        tracker,
	}

	// Register routes.
	s.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range s.routeHandlers(allowedOrigins) {
		s.router.HandleFunc(route, handler)
	}

	// Scheduler successfully initialized.
	return s
}

func (s *Scheduler) RebuildRoutes(allowedOrigins []string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	// Clear existing routes and re-register them.
	s.router = http.NewServeMux()
	// Register routes.
	s.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	for route, handler := range s.routeHandlers(allowedOrigins) {
		s.router.HandleFunc(route, handler)
	}
}

func (s *Scheduler) routeHandlers(allowedOrigins []string) map[string]http.HandlerFunc {
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
		m[route] = inference.CorsMiddleware(allowedOrigins, http.HandlerFunc(s.handleOpenAIInference)).ServeHTTP
		// Register OPTIONS for CORS preflight.
		optionsRoute := "OPTIONS " + route[strings.Index(route, " "):]
		m[optionsRoute] = inference.CorsMiddleware(allowedOrigins, http.HandlerFunc(s.handleOpenAIInference)).ServeHTTP
	}
	m["GET "+inference.InferencePrefix+"/status"] = s.GetBackendStatus
	m["GET "+inference.InferencePrefix+"/ps"] = s.GetRunningBackends
	m["GET "+inference.InferencePrefix+"/df"] = s.GetDiskUsage
	m["POST "+inference.InferencePrefix+"/unload"] = s.Unload
	m["POST "+inference.InferencePrefix+"/{backend}/_configure"] = s.Configure
	m["POST "+inference.InferencePrefix+"/_configure"] = s.Configure
	return m
}

func (s *Scheduler) GetRoutes() []string {
	routeHandlers := s.routeHandlers(nil)
	routes := make([]string, 0, len(routeHandlers))
	for route := range routeHandlers {
		routes = append(routes, route)
	}
	return routes
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
		s.tracker.TrackModel(model)
	}

	// Request a runner to execute the request and defer its release.
	runner, err := s.loader.load(r.Context(), backend.Name(), request.Model, backendMode)
	if err != nil {
		http.Error(w, fmt.Errorf("unable to load runner: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	defer s.loader.release(runner)

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

	for key, slot := range s.loader.runners {
		if s.loader.slots[slot] != nil {
			status := BackendStatus{
				BackendName: key.backend,
				ModelName:   key.model,
				Mode:        key.mode.String(),
				LastUsed:    time.Time{},
			}

			if s.loader.references[slot] == 0 {
				status.LastUsed = s.loader.timestamps[slot]
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
		Model:           "",
		ContextSize:     -1,
		RawRuntimeFlags: "",
	}
	if err := json.Unmarshal(body, &configureRequest); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	rawFlags, err := shellwords.Parse(configureRequest.RawRuntimeFlags)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var runnerConfig inference.BackendConfiguration
	runnerConfig.ContextSize = configureRequest.ContextSize
	runnerConfig.RawFlags = rawFlags

	if err := s.loader.setRunnerConfig(r.Context(), backend.Name(), configureRequest.Model, inference.BackendModeCompletion, runnerConfig); err != nil {
		s.log.Warnf("Failed to configure %s runner for %s: %s", backend.Name(), configureRequest.Model, err)
		if errors.Is(err, errRunnerAlreadyActive) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// ServeHTTP implements net/http.Handler.ServeHTTP.
func (s *Scheduler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.router.ServeHTTP(w, r)
}
