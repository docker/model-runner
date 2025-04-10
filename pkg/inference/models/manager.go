package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"

	"github.com/docker/model-distribution/pkg/distribution"
	"github.com/docker/model-distribution/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/sirupsen/logrus"
)

const (
	// maximumConcurrentModelPulls is the maximum number of concurrent model
	// pulls that a model manager will allow.
	maximumConcurrentModelPulls = 2
)

// Manager manages inference model pulls and storage.
type Manager struct {
	// log is the associated logger.
	log logging.Logger
	// pullTokens is a semaphore used to restrict the maximum number of
	// concurrent pull requests.
	pullTokens chan struct{}
	// router is the HTTP request router.
	router *http.ServeMux
	// distributionClient is the client for model distribution.
	distributionClient *distribution.Client
}

type ClientConfig struct {
	// StoreRootPath is the root path for the model store.
	StoreRootPath string
	// Logger is the logger to use.
	Logger *logrus.Entry
	// Transport is the HTTP transport to use.
	Transport http.RoundTripper
	// UserAgent is the user agent to use.
	UserAgent string
}

// NewManager creates a new model's manager.
func NewManager(log logging.Logger, c ClientConfig) *Manager {
	// Create the model distribution client.
	distributionClient, err := distribution.NewClient(
		distribution.WithStoreRootPath(c.StoreRootPath),
		distribution.WithLogger(c.Logger),
		distribution.WithTransport(c.Transport),
		distribution.WithUserAgent(c.UserAgent),
	)
	if err != nil {
		log.Errorf("Failed to create distribution client: %v", err)
		// Continue without distribution client. The model manager will still
		// respond to requests, but may return errors if the client is required.
	}

	// Create the manager.
	m := &Manager{
		log:                log,
		pullTokens:         make(chan struct{}, maximumConcurrentModelPulls),
		router:             http.NewServeMux(),
		distributionClient: distributionClient,
	}

	// Register routes.
	m.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range m.routeHandlers() {
		m.router.HandleFunc(route, handler)
	}

	// Populate the pull concurrency semaphore.
	for i := 0; i < maximumConcurrentModelPulls; i++ {
		m.pullTokens <- struct{}{}
	}

	// Manager successfully initialized.
	return m
}

func (m *Manager) routeHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST " + inference.ModelsPrefix + "/create":                          m.handleCreateModel,
		"GET " + inference.ModelsPrefix:                                       m.handleGetModels,
		"GET " + inference.ModelsPrefix + "/{name...}":                        m.handleGetModel,
		"DELETE " + inference.ModelsPrefix + "/{name...}":                     m.handleDeleteModel,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models":           m.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models/{name...}": m.handleOpenAIGetModel,
		"GET " + inference.InferencePrefix + "/v1/models":                     m.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/v1/models/{name...}":           m.handleOpenAIGetModel,
	}
}

func (m *Manager) GetRoutes() []string {
	routeHandlers := m.routeHandlers()
	routes := make([]string, 0, len(routeHandlers))
	for route := range routeHandlers {
		routes = append(routes, route)
	}
	return routes
}

// handleCreateModel handles POST <inference-prefix>/models/create requests.
func (m *Manager) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Decode the request.
	var request ModelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Pull the model. In the future, we may support additional operations here
	// besides pulling (such as model building).
	if err := m.PullModel(r.Context(), request.From, w); err != nil {
		if errors.Is(err, distribution.ErrInvalidReference) {
			m.log.Warnf("Invalid model reference %q: %v", request.From, err)
			http.Error(w, "Invalid model reference", http.StatusBadRequest)
			return
		}
		if errors.Is(err, distribution.ErrUnauthorized) || errors.Is(err, distribution.ErrModelNotFound) {
			m.log.Warnf("Failed to pull model %q: %v", request.From, err)
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleGetModels handles GET <inference-prefix>/models requests.
func (m *Manager) handleGetModels(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Query models.
	models, err := m.distributionClient.ListModels()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apiModels := make([]*Model, len(models))
	for i, model := range models {
		apiModels[i], err = ToModel(model)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiModels); err != nil {
		m.log.Warnln("Error while encoding model listing response:", err)
	}
}

// handleGetModel handles GET <inference-prefix>/models/{name} requests.
func (m *Manager) handleGetModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Query the model.
	model, err := m.GetModel(r.PathValue("name"))
	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	apiModel, err := ToModel(model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiModel); err != nil {
		m.log.Warnln("Error while encoding model response:", err)
	}
}

// handleDeleteModel handles DELETE <inference-prefix>/models/{name} requests.
func (m *Manager) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// TODO: We probably want the manager to have a lock / unlock mechanism for
	// models so that active runners can retain / release a model, analogous to
	// a container blocking the release of an image. However, unlike containers,
	// runners are only evicted when idle or when memory is needed, so users
	// won't be able to release the images manually. Perhaps we can unlink the
	// corresponding GGUF files from disk and allow the OS to clean them up once
	// the runner process exits (though this won't work for Windows, where we
	// might need some separate cleanup process).

	err := m.distributionClient.DeleteModel(r.PathValue("name"))
	if err != nil {
		m.log.Warnln("Error while deleting model:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleOpenAIGetModels handles GET <inference-prefix>/<backend>/v1/models and
// GET /<inference-prefix>/v1/models requests.
func (m *Manager) handleOpenAIGetModels(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Query models.
	available, err := m.distributionClient.ListModels()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	models, err := ToOpenAIList(available)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		m.log.Warnln("Error while encoding OpenAI model listing response:", err)
	}
}

// handleOpenAIGetModel handles GET <inference-prefix>/<backend>/v1/models/{name}
// and GET <inference-prefix>/v1/models/{name} requests.
func (m *Manager) handleOpenAIGetModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Query the model.
	model, err := m.GetModel(r.PathValue("name"))
	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	openaiModel, err := ToOpenAI(model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(openaiModel); err != nil {
		m.log.Warnln("Error while encoding OpenAI model response:", err)
	}
}

// ServeHTTP implement net/http.Handler.ServeHTTP.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.router.ServeHTTP(w, r)
}

// GetModel returns a single model.
func (m *Manager) GetModel(ref string) (types.Model, error) {
	model, err := m.distributionClient.GetModel(ref)
	if err != nil {
		return nil, fmt.Errorf("error while getting model: %w", err)
	}
	return model, err
}

// GetModelPath returns the path to a model's files.
func (m *Manager) GetModelPath(ref string) (string, error) {
	model, err := m.GetModel(ref)
	if err != nil {
		return "", err
	}
	path, err := model.GGUFPath()
	if err != nil {
		return "", fmt.Errorf("error while getting model path: %w", err)
	}
	return path, nil
}

// PullModel pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) PullModel(ctx context.Context, model string, w http.ResponseWriter) error {
	// Restrict model pull concurrency.
	select {
	case <-m.pullTokens:
	case <-ctx.Done():
		return context.Canceled
	}
	defer func() {
		m.pullTokens <- struct{}{}
	}()

	// Set up response headers for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Create a flusher to ensure chunks are sent immediately
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Create a progress writer that writes to the response
	progressWriter := &progressResponseWriter{
		writer:  w,
		flusher: flusher,
	}

	// Pull the model using the Docker model distribution client
	m.log.Infoln("Pulling model:", model)
	err := m.distributionClient.PullModel(ctx, model, progressWriter)
	if err != nil {
		return fmt.Errorf("error while pulling model: %w", err)
	}

	return nil
}

// progressResponseWriter implements io.Writer to write progress updates to the HTTP response
type progressResponseWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func (w *progressResponseWriter) Write(p []byte) (n int, err error) {
	escapedData := html.EscapeString(string(p))
	n, err = w.writer.Write([]byte(escapedData))
	if err != nil {
		return 0, err
	}
	// Flush the response to ensure the chunk is sent immediately
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return n, nil
}
