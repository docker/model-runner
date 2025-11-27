package scheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/middleware"
)

// ServeHTTP implements net/http.Handler.ServeHTTP.
func (s *Scheduler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	s.httpHandler.ServeHTTP(w, r)
}

// RebuildRoutes updates the HTTP routes with new allowed origins.
func (s *Scheduler) RebuildRoutes(allowedOrigins []string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	// Update handlers that depend on the allowed origins.
	s.httpHandler = middleware.CorsMiddleware(allowedOrigins, s.router)
}

// routeHandlers returns a map of HTTP routes to their handler functions.
func (s *Scheduler) routeHandlers() map[string]http.HandlerFunc {
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
	m["GET "+inference.InferencePrefix+"/requests"] = s.openAIRecorder.GetRecordsHandler()
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
		model, err := s.modelManager.GetLocal(request.Model)
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
		s.tracker.TrackModel(model, r.UserAgent(), action)

		// Automatically identify models for vLLM.
		backend = s.selectBackendForModel(model, backend, request.Model)
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
		} else if errors.Is(err, vllm.ErrorNotFound) {
			http.Error(w, err.Error(), http.StatusPreconditionFailed)
		} else {
			http.Error(w, fmt.Errorf("backend installation failed: %w", err).Error(), http.StatusServiceUnavailable)
		}
		return
	}

	modelID := s.modelManager.ResolveID(request.Model)

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

// handleModels handles GET /engines/{backend}/v1/models* requests
// by delegating to the model manager
func (s *Scheduler) handleModels(w http.ResponseWriter, r *http.Request) {
	s.modelHandler.ServeHTTP(w, r)
}

// GetBackendStatus returns the status of all backends.
func (s *Scheduler) GetBackendStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]string)
	for backendName, backend := range s.backends {
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
func (s *Scheduler) GetRunningBackends(w http.ResponseWriter, r *http.Request) {
	runningBackends := s.getLoaderStatus(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(runningBackends); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetDiskUsage returns disk usage information for models and backends.
func (s *Scheduler) GetDiskUsage(w http.ResponseWriter, _ *http.Request) {
	modelsDiskUsage, err := s.modelManager.GetDiskUsage()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get models disk usage: %v", err), http.StatusInternalServerError)
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

	unloadedRunners := UnloadResponse{s.loader.Unload(r.Context(), unloadRequest)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(unloadedRunners); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// Configure handles POST <inference-prefix>/{backend}/_configure requests.
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

	_, err = s.ConfigureRunner(r.Context(), backend, configureRequest, r.UserAgent())
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
