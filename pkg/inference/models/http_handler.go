package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
	"github.com/sirupsen/logrus"
)

// HTTPHandler manages inference model pulls and storage.
type HTTPHandler struct {
	// log is the associated logger.
	log logging.Logger
	// router is the HTTP request router.
	router *http.ServeMux
	// httpHandler is the HTTP request handler, which wraps router with
	// the server-level middleware.
	httpHandler http.Handler
	// lock is used to synchronize access to the models manager's router.
	lock sync.RWMutex
	// manager handles business logic for model operations.
	manager *Manager
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
	// PlainHTTP enables plain HTTP connections to registries (for testing).
	PlainHTTP bool
}

// NewHTTPHandler creates a new model's handler.
func NewHTTPHandler(log logging.Logger, manager *Manager, allowedOrigins []string) *HTTPHandler {
	m := &HTTPHandler{
		log:     log,
		router:  http.NewServeMux(),
		manager: manager,
	}

	// Register routes.
	m.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range m.routeHandlers() {
		m.router.HandleFunc(route, handler)
	}

	m.RebuildRoutes(allowedOrigins)

	// HTTPHandler successfully initialized.
	return m
}

func (h *HTTPHandler) RebuildRoutes(allowedOrigins []string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	// Update handlers that depend on the allowed origins.
	h.httpHandler = middleware.CorsMiddleware(allowedOrigins, h.router)
}

func (h *HTTPHandler) routeHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST " + inference.ModelsPrefix + "/create":                          h.handleCreateModel,
		"POST " + inference.ModelsPrefix + "/load":                            h.handleLoadModel,
		"GET " + inference.ModelsPrefix:                                       h.handleGetModels,
		"GET " + inference.ModelsPrefix + "/{name...}":                        h.handleGetModel,
		"DELETE " + inference.ModelsPrefix + "/{name...}":                     h.handleDeleteModel,
		"POST " + inference.ModelsPrefix + "/{nameAndAction...}":              h.handleModelAction,
		"DELETE " + inference.ModelsPrefix + "/purge":                         h.handlePurge,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models":           h.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models/{name...}": h.handleOpenAIGetModel,
		"GET " + inference.InferencePrefix + "/v1/models":                     h.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/v1/models/{name...}":           h.handleOpenAIGetModel,
	}
}

// handleCreateModel handles POST <inference-prefix>/models/create requests.
func (h *HTTPHandler) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	// Decode the request.
	var request ModelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Pull the model
	if err := h.manager.Pull(request.From, request.BearerToken, r, w); err != nil {
		sanitizedFrom := utils.SanitizeForLog(request.From, -1)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			h.log.Infof("Request canceled/timed out while pulling model %q", sanitizedFrom)
			return
		}
		if errors.Is(err, registry.ErrInvalidReference) {
			h.log.Warnf("Invalid model reference %q: %v", sanitizedFrom, err)
			http.Error(w, "Invalid model reference", http.StatusBadRequest)
			return
		}
		if errors.Is(err, registry.ErrUnauthorized) {
			h.log.Warnf("Unauthorized to pull model %q: %v", sanitizedFrom, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, registry.ErrModelNotFound) {
			h.log.Warnf("Failed to pull model %q: %v", sanitizedFrom, err)
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}
		// Note: ErrUnsupportedFormat is no longer treated as an error - it's a warning
		// that's sent to the client via the progress stream
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLoadModel handles POST <inference-prefix>/models/load requests.
func (h *HTTPHandler) handleLoadModel(w http.ResponseWriter, r *http.Request) {
	err := h.manager.Load(r.Body, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleGetModels handles GET <inference-prefix>/models requests.
func (h *HTTPHandler) handleGetModels(w http.ResponseWriter, r *http.Request) {
	apiModels, err := h.manager.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiModels); err != nil {
		h.log.Warnln("Error while encoding model listing response:", err)
	}
}

// handleGetModel handles GET <inference-prefix>/models/{name} requests.
func (h *HTTPHandler) handleGetModel(w http.ResponseWriter, r *http.Request) {
	modelRef := r.PathValue("name")

	// Parse remote query parameter
	remote := false
	if r.URL.Query().Has("remote") {
		val, err := strconv.ParseBool(r.URL.Query().Get("remote"))
		if err != nil {
			h.log.Warnln("Error while parsing remote query parameter:", err)
		} else {
			remote = val
		}
	}

	var (
		apiModel *Model
		err      error
	)

	if remote {
		apiModel, err = h.getRemoteAPIModel(r.Context(), modelRef)
	} else {
		apiModel, err = h.getLocalAPIModel(modelRef)
	}

	if err != nil {
		h.writeModelError(w, err)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiModel); err != nil {
		h.log.Warnln("Error while encoding model response:", err)
	}
}

func (h *HTTPHandler) getRemoteAPIModel(ctx context.Context, modelRef string) (*Model, error) {
	model, err := h.manager.GetRemote(ctx, modelRef)
	if err != nil {
		return nil, err
	}
	return ToModelFromArtifact(model)
}

func (h *HTTPHandler) getLocalAPIModel(modelRef string) (*Model, error) {
	model, err := h.manager.GetLocal(modelRef)
	if err != nil {
		// If not found locally, try partial name matching
		if errors.Is(err, distribution.ErrModelNotFound) {
			// e.g., "smollm2" for "ai/smollm2:latest"
			return findModelByPartialName(h, modelRef)
		}
		return nil, err
	}

	return ToModel(model)
}

func (h *HTTPHandler) writeModelError(w http.ResponseWriter, err error) {
	if errors.Is(err, distribution.ErrModelNotFound) || errors.Is(err, registry.ErrModelNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// findModelByPartialName looks for a model by matching the provided reference
// against model tags using partial name matching (e.g., "smollm2" matches "ai/smollm2:latest")
func findModelByPartialName(h *HTTPHandler, modelRef string) (*Model, error) {
	// Get all models to search through their tags
	models, err := h.manager.RawList()
	if err != nil {
		return nil, err
	}

	// Look for a model whose tags match the reference
	for _, model := range models {
		for _, tag := range model.Tags() {
			// Extract the model name without tag part (e.g., from "ai/smollm2:latest" get "ai/smollm2")
			tagWithoutVersion := tag
			if idx := strings.LastIndex(tag, ":"); idx != -1 {
				tagWithoutVersion = tag[:idx]
			}

			// Get just the name part without organization (e.g., from "ai/smollm2" get "smollm2")
			namePart := tagWithoutVersion
			if idx := strings.LastIndex(tagWithoutVersion, "/"); idx != -1 {
				namePart = tagWithoutVersion[idx+1:]
			}

			// Check if the reference matches the name part
			if namePart == modelRef {
				return ToModel(model)
			}
		}
	}

	return nil, distribution.ErrModelNotFound
}

// handleDeleteModel handles DELETE <inference-prefix>/models/{name} requests.
// query params:
// - force: if true, delete the model even if it has multiple tags
func (h *HTTPHandler) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	// TODO: We probably want the manager to have a lock / unlock mechanism for
	// models so that active runners can retain / release a model, analogous to
	// a container blocking the release of an image. However, unlike containers,
	// runners are only evicted when idle or when memory is needed, so users
	// won't be able to release the images manually. Perhaps we can unlink the
	// corresponding GGUF files from disk and allow the OS to clean them up once
	// the runner process exits (though this won't work for Windows, where we
	// might need some separate cleanup process).

	modelRef := r.PathValue("name")

	var force bool
	if r.URL.Query().Has("force") {
		if val, err := strconv.ParseBool(r.URL.Query().Get("force")); err != nil {
			h.log.Warnln("Error while parsing force query parameter:", err)
		} else {
			force = val
		}
	}

	// First try to delete without normalization (as ID), then with normalization if not found
	resp, err := h.manager.Delete(modelRef, force)
	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, distribution.ErrConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		h.log.Warnln("Error while deleting model:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("error writing response: %v", err), http.StatusInternalServerError)
	}
}

// handleOpenAIGetModels handles GET <inference-prefix>/<backend>/v1/models and
// GET /<inference-prefix>/v1/models requests.
func (h *HTTPHandler) handleOpenAIGetModels(w http.ResponseWriter, r *http.Request) {
	// Query models.
	available, err := h.manager.RawList()
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
		h.log.Warnln("Error while encoding OpenAI model listing response:", err)
	}
}

// handleOpenAIGetModel handles GET <inference-prefix>/<backend>/v1/models/{name}
// and GET <inference-prefix>/v1/models/{name} requests.
func (h *HTTPHandler) handleOpenAIGetModel(w http.ResponseWriter, r *http.Request) {
	modelRef := r.PathValue("name")
	model, err := h.manager.GetLocal(modelRef)
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
		h.log.Warnln("Error while encoding OpenAI model response:", err)
	}
}

// handleTagModel handles POST <inference-prefix>/models/{nameAndAction} requests.
// Action is one of:
// - tag: tag the model with a repository and tag (e.g. POST <inference-prefix>/models/my-org/my-repo:latest/tag})
// - push: pushes a tagged model to the registry
func (h *HTTPHandler) handleModelAction(w http.ResponseWriter, r *http.Request) {
	model, action := path.Split(r.PathValue("nameAndAction"))
	model = strings.TrimRight(model, "/")

	switch action {
	case "tag":
		h.handleTagModel(w, r, model)
	case "push":
		h.handlePushModel(w, r, model)
	default:
		http.Error(w, fmt.Sprintf("unknown action %q", action), http.StatusNotFound)
	}
}

// handleTagModel handles POST <inference-prefix>/models/{name}/tag requests.
// The query parameters are:
// - repo: the repository to tag the model with (required)
// - tag: the tag to apply to the model (required)
func (h *HTTPHandler) handleTagModel(w http.ResponseWriter, r *http.Request, model string) {
	// Extract query parameters.
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")

	// Validate query parameters.
	if repo == "" || tag == "" {
		http.Error(w, "missing repo or tag query parameter", http.StatusBadRequest)
		return
	}

	// Construct the target string.
	target := fmt.Sprintf("%s:%s", repo, tag)

	// First try to tag using the provided model reference as-is
	err := h.manager.Tag(model, target)
	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		// If there's an error other than not found, return it
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with success.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := map[string]string{
		"message": fmt.Sprintf("Model tagged successfully with %q", target),
		"target":  target,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Warnln("Error while encoding tag response:", err)
	}
}

// handlePushModel handles POST <inference-prefix>/models/{name}/push requests.
func (h *HTTPHandler) handlePushModel(w http.ResponseWriter, r *http.Request, model string) {
	if err := h.manager.Push(model, r, w); err != nil {
		if errors.Is(err, distribution.ErrInvalidReference) {
			h.log.Warnf("Invalid model reference %q: %v", utils.SanitizeForLog(model, -1), err)
			http.Error(w, "Invalid model reference", http.StatusBadRequest)
			return
		}
		if errors.Is(err, distribution.ErrModelNotFound) {
			h.log.Warnf("Failed to push model %q: %v", utils.SanitizeForLog(model, -1), err)
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, registry.ErrUnauthorized) {
			h.log.Warnf("Unauthorized to push model %q: %v", utils.SanitizeForLog(model, -1), err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handlePurge handles DELETE <inference-prefix>/models/purge requests.
func (h *HTTPHandler) handlePurge(w http.ResponseWriter, _ *http.Request) {
	err := h.manager.Purge()
	if err != nil {
		h.log.Warnf("Failed to purge models: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ServeHTTP implement net/http.HTTPHandler.ServeHTTP.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	h.httpHandler.ServeHTTP(w, r)
}

// progressResponseWriter implements io.Writer to write progress updates to the HTTP response
type progressResponseWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	isJSON  bool
}

func (w *progressResponseWriter) Write(p []byte) (n int, err error) {
	var data []byte
	if w.isJSON {
		// For JSON, write the raw bytes without escaping
		data = p
	} else {
		// For plain text, escape HTML
		escapedData := html.EscapeString(string(p))
		data = []byte(escapedData)
	}

	n, err = w.writer.Write(data)
	if err != nil {
		return 0, err
	}
	// Flush the response to ensure the chunk is sent immediately
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return n, nil
}
