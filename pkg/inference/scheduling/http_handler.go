package scheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/middleware"
)

// HTTPHandler handles HTTP requests for the scheduler.
// It wraps the Scheduler to provide HTTP endpoint functionality without
// coupling the core scheduling logic to HTTP concerns.
type HTTPHandler struct {
	scheduler   *Scheduler
	router      *http.ServeMux
	httpHandler http.Handler
	// modelHandler is the shared model handler.
	modelHandler *models.HTTPHandler
	lock         sync.RWMutex
}

// NewHTTPHandler creates a new HTTP handler that wraps the scheduler.
// This is the primary HTTP interface for the scheduling package.
func NewHTTPHandler(s *Scheduler, modelHandler *models.HTTPHandler, allowedOrigins []string) *HTTPHandler {
	h := &HTTPHandler{
		scheduler:    s,
		modelHandler: modelHandler,
		router:       http.NewServeMux(),
	}

	// Register routes
	h.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range h.routeHandlers() {
		h.router.HandleFunc(route, handler)
	}

	h.RebuildRoutes(allowedOrigins)

	return h
}

// routeHandlers returns a map of HTTP routes to their handler functions.
func (h *HTTPHandler) routeHandlers() map[string]http.HandlerFunc {
	openAIRoutes := []string{
		"POST " + inference.InferencePrefix + "/{backend}/v1/chat/completions",
		"POST " + inference.InferencePrefix + "/{backend}/v1/completions",
		"POST " + inference.InferencePrefix + "/{backend}/v1/embeddings",
		"POST " + inference.InferencePrefix + "/v1/chat/completions",
		"POST " + inference.InferencePrefix + "/v1/completions",
		"POST " + inference.InferencePrefix + "/v1/embeddings",
		"POST " + inference.InferencePrefix + "/{backend}/rerank",
		"POST " + inference.InferencePrefix + "/rerank",
		"POST " + inference.InferencePrefix + "/{backend}/score",
		"POST " + inference.InferencePrefix + "/score",
	}
	m := make(map[string]http.HandlerFunc)
	for _, route := range openAIRoutes {
		m[route] = h.handleOpenAIInference
	}

	// Register /v1/models routes - these delegate to the model manager
	m["GET "+inference.InferencePrefix+"/{backend}/v1/models"] = h.handleModels
	m["GET "+inference.InferencePrefix+"/{backend}/v1/models/{name...}"] = h.handleModels
	m["GET "+inference.InferencePrefix+"/v1/models"] = h.handleModels
	m["GET "+inference.InferencePrefix+"/v1/models/{name...}"] = h.handleModels

	m["GET "+inference.InferencePrefix+"/status"] = h.GetBackendStatus
	m["GET "+inference.InferencePrefix+"/ps"] = h.GetRunningBackends
	m["GET "+inference.InferencePrefix+"/df"] = h.GetDiskUsage
	m["POST "+inference.InferencePrefix+"/unload"] = h.Unload
	m["POST "+inference.InferencePrefix+"/{backend}/_configure"] = h.Configure
	m["POST "+inference.InferencePrefix+"/_configure"] = h.Configure
	m["GET "+inference.InferencePrefix+"/requests"] = h.scheduler.openAIRecorder.GetRecordsHandler()
	return m
}

// handleOpenAIInference handles scheduling and responding to OpenAI inference
// requests, including:
// - POST <inference-prefix>/{backend}/v1/chat/completions
// - POST <inference-prefix>/{backend}/v1/completions
// - POST <inference-prefix>/{backend}/v1/embeddings
// and 2 extras:
// - POST <inference-prefix>/{backend}/rerank
// - POST <inference-prefix>/{backend}/score
func (h *HTTPHandler) handleOpenAIInference(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	var backend inference.Backend
	if b := r.PathValue("backend"); b == "" {
		backend = h.scheduler.defaultBackend
	} else {
		backend = h.scheduler.backends[b]
	}
	if backend == nil {
		http.Error(w, ErrBackendNotFound.Error(), http.StatusNotFound)
		return
	}

	// Read the entire request body. We put some basic size constraints in place
	// to avoid DoS attacks. We do this early to avoid client write timeouts.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
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
		model, err := h.scheduler.modelManager.GetLocal(request.Model)
		if err != nil {
			if errors.Is(err, distribution.ErrModelNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, "model unavailable", http.StatusInternalServerError)
			}
			return
		}
		// Determine the action for tracking
		action := "inference/" + backendMode.String()
		// Check if there's a request origin header to provide more specific tracking
		// Only trust whitelisted values to prevent header spoofing
		if origin := r.Header.Get(inference.RequestOriginHeader); origin != "" {
			switch origin {
			case inference.OriginOllamaCompletion:
				action = origin
				// If an unknown origin is provided, ignore it and use the default action
				// This prevents untrusted clients from spoofing tracking data
			}
		}

		// Non-blocking call to track the model usage.
		h.scheduler.tracker.TrackModel(model, r.UserAgent(), action)

		// Automatically identify models for vLLM.
		backend = h.scheduler.selectBackendForModel(model, backend, request.Model)
	}

	// Wait for the corresponding backend installation to complete or fail. We
	// don't allow any requests to be scheduled for a backend until it has
	// completed installation.
	if err := h.scheduler.installer.wait(r.Context(), backend.Name()); err != nil {
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
		} else if errors.Is(err, vllm.ErrorNotFound) {
			http.Error(w, err.Error(), http.StatusPreconditionFailed)
		} else {
			http.Error(w, fmt.Errorf("backend installation failed: %w", err).Error(), http.StatusServiceUnavailable)
		}
		return
	}

	modelID := h.scheduler.modelManager.ResolveID(request.Model)

	// Request a runner to execute the request and defer its release.
	runner, err := h.scheduler.loader.load(r.Context(), backend.Name(), modelID, request.Model, backendMode)
	if err != nil {
		http.Error(w, fmt.Errorf("unable to load runner: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	defer h.scheduler.loader.release(runner)

	// Record the request in the OpenAI recorder.
	recordID := h.scheduler.openAIRecorder.RecordRequest(request.Model, r, body)
	w = h.scheduler.openAIRecorder.NewResponseRecorder(w)
	defer func() {
		// Record the response in the OpenAI recorder.
		h.scheduler.openAIRecorder.RecordResponse(recordID, request.Model, w)
	}()

	// Create a request with the body replaced for forwarding upstream.
	upstreamRequest := r.Clone(r.Context())
	upstreamRequest.Body = io.NopCloser(bytes.NewReader(body))

	// Perform the request.
	runner.ServeHTTP(w, upstreamRequest)
}

// handleModels handles GET /engines/{backend}/v1/models* requests
// by delegating to the model manager
func (h *HTTPHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	h.modelHandler.ServeHTTP(w, r)
}

// GetBackendStatus returns the status of all backends.
func (h *HTTPHandler) GetBackendStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]string)
	for backendName, backend := range h.scheduler.backends {
		status[backendName] = backend.Status()
	}

	data, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// GetRunningBackends returns information about all running backends
func (h *HTTPHandler) GetRunningBackends(w http.ResponseWriter, r *http.Request) {
	runningBackends := h.scheduler.getLoaderStatus(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(runningBackends); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetDiskUsage returns disk usage information for models and backends.
func (h *HTTPHandler) GetDiskUsage(w http.ResponseWriter, _ *http.Request) {
	modelsDiskUsage, err := h.scheduler.modelManager.GetDiskUsage()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get models disk usage: %v", err), http.StatusInternalServerError)
		return
	}

	// TODO: Get disk usage for each backend once the backends are implemented.
	defaultBackendDiskUsage, err := h.scheduler.defaultBackend.GetDiskUsage()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get disk usage for %s: %v", h.scheduler.defaultBackend.Name(), err), http.StatusInternalServerError)
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
func (h *HTTPHandler) Unload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
		}
		return
	}

	var unloadRequest UnloadRequest
	if err := json.Unmarshal(body, &unloadRequest); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	unloadedRunners := UnloadResponse{h.scheduler.loader.Unload(r.Context(), unloadRequest)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(unloadedRunners); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// Configure handles POST <inference-prefix>/{backend}/_configure requests.
func (h *HTTPHandler) Configure(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	var backend inference.Backend
	if b := r.PathValue("backend"); b == "" {
		backend = h.scheduler.defaultBackend
	} else {
		backend = h.scheduler.backends[b]
	}
	if backend == nil {
		http.Error(w, ErrBackendNotFound.Error(), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOpenAIInferenceRequestSize))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
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

	_, err = h.scheduler.ConfigureRunner(r.Context(), backend, configureRequest, r.UserAgent())
	if err != nil {
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
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	h.httpHandler.ServeHTTP(w, r)
}

// RebuildRoutes updates the HTTP routes with new allowed origins.
func (h *HTTPHandler) RebuildRoutes(allowedOrigins []string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	// Update handlers that depend on the allowed origins.
	h.httpHandler = middleware.CorsMiddleware(allowedOrigins, h.router)
}

// GetLlamaCppSocket delegates to the scheduler's business logic.
// Required by metrics.SchedulerInterface.
func (h *HTTPHandler) GetLlamaCppSocket() (string, error) {
	return h.scheduler.GetLlamaCppSocket()
}

// GetAllActiveRunners delegates to the scheduler's business logic.
// Required by metrics.SchedulerInterface.
func (h *HTTPHandler) GetAllActiveRunners() []metrics.ActiveRunner {
	return h.scheduler.GetAllActiveRunners()
}
