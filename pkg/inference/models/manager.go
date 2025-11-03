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

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sirupsen/logrus"
)

const (
	// maximumConcurrentModelPulls is the maximum number of concurrent model
	// pulls that a model manager will allow.
	maximumConcurrentModelPulls = 2
	defaultOrg                  = "ai"
	defaultTag                  = "latest"
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
	// httpHandler is the HTTP request handler, which wraps router with
	// the server-level middleware.
	httpHandler http.Handler
	// distributionClient is the client for model distribution.
	distributionClient *distribution.Client
	// registryClient is the client for model registry.
	registryClient *registry.Client
	// lock is used to synchronize access to the models manager's router.
	lock sync.RWMutex
	// memoryEstimator is used to calculate runtime memory requirements for models.
	memoryEstimator memory.MemoryEstimator
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
func NewManager(log logging.Logger, c ClientConfig, allowedOrigins []string, memoryEstimator memory.MemoryEstimator) *Manager {
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

	// Create the model registry client.
	registryClient := registry.NewClient(
		registry.WithTransport(c.Transport),
		registry.WithUserAgent(c.UserAgent),
	)

	// Create the manager.
	m := &Manager{
		log:                log,
		pullTokens:         make(chan struct{}, maximumConcurrentModelPulls),
		router:             http.NewServeMux(),
		distributionClient: distributionClient,
		registryClient:     registryClient,
		memoryEstimator:    memoryEstimator,
	}

	// Register routes.
	m.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	for route, handler := range m.routeHandlers() {
		m.router.HandleFunc(route, handler)
	}

	m.RebuildRoutes(allowedOrigins)

	// Populate the pull concurrency semaphore.
	for i := 0; i < maximumConcurrentModelPulls; i++ {
		m.pullTokens <- struct{}{}
	}

	// Manager successfully initialized.
	return m
}

func (m *Manager) RebuildRoutes(allowedOrigins []string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	// Update handlers that depend on the allowed origins.
	m.httpHandler = middleware.CorsMiddleware(allowedOrigins, m.router)
}

// NormalizeModelName adds the default organization prefix (ai/) and tag (:latest) if missing.
// It also converts Hugging Face model names to lowercase.
// Examples:
//   - "gemma3" -> "ai/gemma3:latest"
//   - "gemma3:v1" -> "ai/gemma3:v1"
//   - "myorg/gemma3" -> "myorg/gemma3:latest"
//   - "ai/gemma3:latest" -> "ai/gemma3:latest" (unchanged)
//   - "hf.co/model" -> "hf.co/model:latest" (unchanged - has registry)
//   - "hf.co/Model" -> "hf.co/model:latest" (converted to lowercase)
func NormalizeModelName(model string) string {
	// If the model is empty, return as-is
	if model == "" {
		return model
	}

	// Normalize HuggingFace model names (lowercase)
	if strings.HasPrefix(model, "hf.co/") {
		model = strings.ToLower(model)
	}

	// Check if model contains a registry (domain with dot before first slash)
	firstSlash := strings.Index(model, "/")
	if firstSlash > 0 && strings.Contains(model[:firstSlash], ".") {
		// Has a registry, just ensure tag
		if !strings.Contains(model, ":") {
			return model + ":" + defaultTag
		}
		return model
	}

	// Split by colon to check for tag
	parts := strings.SplitN(model, ":", 2)
	nameWithOrg := parts[0]
	tag := defaultTag
	if len(parts) == 2 {
		tag = parts[1]
	}

	// If name doesn't contain a slash, add the default org
	if !strings.Contains(nameWithOrg, "/") {
		nameWithOrg = defaultOrg + "/" + nameWithOrg
	}

	return nameWithOrg + ":" + tag
}

func (m *Manager) routeHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST " + inference.ModelsPrefix + "/create":                          m.handleCreateModel,
		"POST " + inference.ModelsPrefix + "/load":                            m.handleLoadModel,
		"POST " + inference.ModelsPrefix + "/package":                         m.handlePackageModel,
		"GET " + inference.ModelsPrefix:                                       m.handleGetModels,
		"GET " + inference.ModelsPrefix + "/{name...}":                        m.handleGetModel,
		"DELETE " + inference.ModelsPrefix + "/{name...}":                     m.handleDeleteModel,
		"POST " + inference.ModelsPrefix + "/{nameAndAction...}":              m.handleModelAction,
		"DELETE " + inference.ModelsPrefix + "/purge":                         m.handlePurge,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models":           m.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/{backend}/v1/models/{name...}": m.handleOpenAIGetModel,
		"GET " + inference.InferencePrefix + "/v1/models":                     m.handleOpenAIGetModels,
		"GET " + inference.InferencePrefix + "/v1/models/{name...}":           m.handleOpenAIGetModel,
	}
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

	// Normalize the model name to add defaults
	request.From = NormalizeModelName(request.From)

	// Pull the model. In the future, we may support additional operations here
	// besides pulling (such as model building).
	if memory.RuntimeMemoryCheckEnabled() && !request.IgnoreRuntimeMemoryCheck {
		m.log.Infof("Will estimate memory required for %q", request.From)
		proceed, req, totalMem, err := m.memoryEstimator.HaveSufficientMemoryForModel(r.Context(), request.From, nil)
		if err != nil {
			m.log.Warnf("Failed to validate sufficient system memory for model %q: %s", request.From, err)
			// Prefer staying functional in case of unexpected estimation errors.
			proceed = true
		}
		if !proceed {
			errstr := fmt.Sprintf("Runtime memory requirement for model %q exceeds total system memory: required %d RAM %d VRAM, system %d RAM %d VRAM", request.From, req.RAM, req.VRAM, totalMem.RAM, totalMem.VRAM)
			m.log.Warnf(errstr)
			http.Error(w, errstr, http.StatusInsufficientStorage)
			return
		}
	}
	if err := m.PullModel(request.From, r, w); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			m.log.Infof("Request canceled/timed out while pulling model %q", request.From)
			return
		}
		if errors.Is(err, registry.ErrInvalidReference) {
			m.log.Warnf("Invalid model reference %q: %v", request.From, err)
			http.Error(w, "Invalid model reference", http.StatusBadRequest)
			return
		}
		if errors.Is(err, registry.ErrUnauthorized) {
			m.log.Warnf("Unauthorized to pull model %q: %v", request.From, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, registry.ErrModelNotFound) {
			m.log.Warnf("Failed to pull model %q: %v", request.From, err)
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, distribution.ErrUnsupportedFormat) {
			m.log.Warnf("Unsupported model format for %q: %v", request.From, err)
			http.Error(w, distribution.ErrUnsupportedFormat.Error(), http.StatusUnsupportedMediaType)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLoadModel handles POST <inference-prefix>/models/load requests.
func (m *Manager) handleLoadModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	if _, err := m.distributionClient.LoadModel(r.Body, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	return
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
	modelRef := r.PathValue("name")

	// Parse remote query parameter
	remote := false
	if r.URL.Query().Has("remote") {
		if val, err := strconv.ParseBool(r.URL.Query().Get("remote")); err != nil {
			m.log.Warnln("Error while parsing remote query parameter:", err)
		} else {
			remote = val
		}
	}

	if remote && m.registryClient == nil {
		http.Error(w, "registry client unavailable", http.StatusServiceUnavailable)
		return
	}

	var apiModel *Model
	var err error

	if remote {
		// For remote lookups, always normalize the reference
		normalizedRef := NormalizeModelName(modelRef)
		apiModel, err = getRemoteModel(r.Context(), m, normalizedRef)
	} else {
		// For local lookups, first try without normalization (as ID), then with normalization
		apiModel, err = getLocalModel(m, modelRef)
		if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
			// If not found as-is, try with normalization
			normalizedRef := NormalizeModelName(modelRef)
			if normalizedRef != modelRef { // only try normalized if it's different
				apiModel, err = getLocalModel(m, normalizedRef)
			}
		}

		// If still not found, try partial name matching (e.g., "smollm2" for "ai/smollm2:latest")
		if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
			apiModel, err = findModelByPartialName(m, modelRef)
		}
	}

	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) || errors.Is(err, registry.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiModel); err != nil {
		m.log.Warnln("Error while encoding model response:", err)
	}
}

// ResolveModelID resolves a model reference to a model ID. If resolution fails, it returns the original ref.
func (m *Manager) ResolveModelID(modelRef string) string {
	// Sanitize modelRef to prevent log forgery
	sanitizedModelRef := strings.ReplaceAll(modelRef, "\n", "")
	sanitizedModelRef = strings.ReplaceAll(sanitizedModelRef, "\r", "")

	model, err := m.GetModel(sanitizedModelRef)
	if err != nil {
		m.log.Warnf("Failed to resolve model ref %s to ID: %v", sanitizedModelRef, err)
		return sanitizedModelRef
	}

	modelID, err := model.ID()
	if err != nil {
		m.log.Warnf("Failed to get model ID for ref %s: %v", sanitizedModelRef, err)
		return sanitizedModelRef
	}

	return modelID
}

func getLocalModel(m *Manager, name string) (*Model, error) {
	if m.distributionClient == nil {
		return nil, errors.New("model distribution service unavailable")
	}

	// Query the model.
	model, err := m.GetModel(name)
	if err != nil {
		return nil, err
	}

	return ToModel(model)
}

func getRemoteModel(ctx context.Context, m *Manager, name string) (*Model, error) {
	if m.registryClient == nil {
		return nil, errors.New("registry client unavailable")
	}

	m.log.Infoln("Getting remote model:", name)
	model, err := m.registryClient.Model(ctx, name)
	if err != nil {
		return nil, err
	}

	id, err := model.ID()
	if err != nil {
		return nil, err
	}

	descriptor, err := model.Descriptor()
	if err != nil {
		return nil, err
	}

	config, err := model.Config()
	if err != nil {
		return nil, err
	}

	apiModel := &Model{
		ID:      id,
		Tags:    nil,
		Created: descriptor.Created.Unix(),
		Config:  config,
	}

	return apiModel, nil
}

// findModelByPartialName looks for a model by matching the provided reference
// against model tags using partial name matching (e.g., "smollm2" matches "ai/smollm2:latest")
func findModelByPartialName(m *Manager, modelRef string) (*Model, error) {
	// Get all models to search through their tags
	models, err := m.distributionClient.ListModels()
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

	modelRef := r.PathValue("name")

	var force bool
	if r.URL.Query().Has("force") {
		if val, err := strconv.ParseBool(r.URL.Query().Get("force")); err != nil {
			m.log.Warnln("Error while parsing force query parameter:", err)
		} else {
			force = val
		}
	}

	// First try to delete without normalization (as ID), then with normalization if not found
	resp, err := m.distributionClient.DeleteModel(modelRef, force)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// If not found as-is, try with normalization
		normalizedRef := NormalizeModelName(modelRef)
		if normalizedRef != modelRef { // only try normalized if it's different
			resp, err = m.distributionClient.DeleteModel(normalizedRef, force)
		}
	}

	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, distribution.ErrConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		m.log.Warnln("Error while deleting model:", err)
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

	modelRef := r.PathValue("name")

	// Query the model - first try without normalization (as ID), then with normalization
	model, err := m.GetModel(modelRef)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// If not found as-is, try with normalization
		normalizedRef := NormalizeModelName(modelRef)
		if normalizedRef != modelRef { // only try normalized if it's different
			model, err = m.GetModel(normalizedRef)
		}
	}

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

// handleTagModel handles POST <inference-prefix>/models/{nameAndAction} requests.
// Action is one of:
// - tag: tag the model with a repository and tag (e.g. POST <inference-prefix>/models/my-org/my-repo:latest/tag})
// - push: pushes a tagged model to the registry
func (m *Manager) handleModelAction(w http.ResponseWriter, r *http.Request) {
	model, action := path.Split(r.PathValue("nameAndAction"))
	model = strings.TrimRight(model, "/")

	// For tag and push actions, we likely expect model references rather than IDs,
	// so normalize the model name, but we'll handle both cases in the handlers
	normalizedModel := NormalizeModelName(model)
	switch action {
	case "tag":
		m.handleTagModel(w, r, normalizedModel)
	case "push":
		m.handlePushModel(w, r, normalizedModel)
	default:
		http.Error(w, fmt.Sprintf("unknown action %q", action), http.StatusNotFound)
	}
}

// handleTagModel handles POST <inference-prefix>/models/{name}/tag requests.
// The query parameters are:
// - repo: the repository to tag the model with (required)
// - tag: the tag to apply to the model (required)
func (m *Manager) handleTagModel(w http.ResponseWriter, r *http.Request, model string) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

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
	err := m.distributionClient.Tag(model, target)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// Check if the model parameter is a model ID (starts with sha256:) or is a partial name
		var foundModelRef string
		found := false

		// If it looks like an ID, try to find the model by ID
		if strings.HasPrefix(model, "sha256:") || len(model) == 12 { // 12-char short ID
			// Get all models and find the one matching this ID
			models, listErr := m.distributionClient.ListModels()
			if listErr != nil {
				http.Error(w, fmt.Sprintf("error listing models: %v", listErr), http.StatusInternalServerError)
				return
			}

			for _, mModel := range models {
				modelID, idErr := mModel.ID()
				if idErr != nil {
					m.log.Warnf("Failed to get model ID: %v", idErr)
					continue
				}

				// Check if the model ID matches (can be full or short ID)
				if modelID == model || strings.HasPrefix(modelID, model) {
					// Use the first tag of this model as the source reference
					tags := mModel.Tags()
					if len(tags) > 0 {
						foundModelRef = tags[0]
						found = true
						break
					}
				}
			}
		}

		// If not found by ID, try partial name matching (similar to inspect)
		if !found {
			models, listErr := m.distributionClient.ListModels()
			if listErr != nil {
				http.Error(w, fmt.Sprintf("error listing models: %v", listErr), http.StatusInternalServerError)
				return
			}

			// Look for a model whose tags match the provided reference
			for _, mModel := range models {
				for _, tagStr := range mModel.Tags() {
					// Extract the model name without tag part (e.g., from "ai/smollm2:latest" get "ai/smollm2")
					tagWithoutVersion := tagStr
					if idx := strings.LastIndex(tagStr, ":"); idx != -1 {
						tagWithoutVersion = tagStr[:idx]
					}

					// Get just the name part without organization (e.g., from "ai/smollm2" get "smollm2")
					namePart := tagWithoutVersion
					if idx := strings.LastIndex(tagWithoutVersion, "/"); idx != -1 {
						namePart = tagWithoutVersion[idx+1:]
					}

					// Check if the provided model matches the name part
					if namePart == model {
						// Found a match - use the tag string that matched as the source reference
						foundModelRef = tagStr
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}

		if !found {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Now tag using the found model reference (the matching tag)
		if tagErr := m.distributionClient.Tag(foundModelRef, target); tagErr != nil {
			m.log.Warnf("Failed to apply tag %q to resolved model %q: %v", target, foundModelRef, tagErr)
			http.Error(w, tagErr.Error(), http.StatusInternalServerError)
			return
		}
	} else if err != nil {
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
		m.log.Warnln("Error while encoding tag response:", err)
	}
}

// handlePushModel handles POST <inference-prefix>/models/{name}/push requests.
func (m *Manager) handlePushModel(w http.ResponseWriter, r *http.Request, model string) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Call the PushModel method on the distribution client.
	if err := m.PushModel(model, r, w); err != nil {
		if errors.Is(err, distribution.ErrInvalidReference) {
			m.log.Warnf("Invalid model reference %q: %v", model, err)
			http.Error(w, "Invalid model reference", http.StatusBadRequest)
			return
		}
		if errors.Is(err, distribution.ErrModelNotFound) {
			m.log.Warnf("Failed to push model %q: %v", model, err)
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, registry.ErrUnauthorized) {
			m.log.Warnf("Unauthorized to push model %q: %v", model, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handlePackageModel handles POST <inference-prefix>/models/package requests.
func (m *Manager) handlePackageModel(w http.ResponseWriter, r *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Decode the request
	var request ModelPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.From == "" || request.Tag == "" {
		http.Error(w, "both 'from' and 'tag' fields are required", http.StatusBadRequest)
		return
	}

	// Normalize the source model name
	request.From = NormalizeModelName(request.From)

	// Create a builder from an existing model by getting the bundle first
	// Since ModelArtifact interface is needed to work with the builder
	bundle, err := m.distributionClient.GetBundle(request.From)
	if err != nil {
		if errors.Is(err, distribution.ErrModelNotFound) {
			http.Error(w, fmt.Sprintf("source model not found: %s", request.From), http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("error getting source model bundle %s: %v", request.From, err), http.StatusInternalServerError)
		}
		return
	}

	// Create a builder from the existing model artifact (from the bundle)
	modelArtifact, ok := bundle.(types.ModelArtifact)
	if !ok {
		http.Error(w, "source model does not implement ModelArtifact interface", http.StatusInternalServerError)
		return
	}

	// Create a builder from the existing model
	bldr, err := builder.FromModel(modelArtifact)
	if err != nil {
		http.Error(w, fmt.Sprintf("error creating builder from model: %v", err), http.StatusInternalServerError)
		return
	}

	// Apply context size if specified
	if request.ContextSize > 0 {
		bldr = bldr.WithContextSize(request.ContextSize)
	}

	// Get the built model artifact
	builtModel := bldr.Model()

	// Check if we can use lightweight repackaging (config-only changes from existing model)
	useLightweight := bldr.HasOnlyConfigChanges()

	if useLightweight {
		// Use the lightweight method to avoid re-transferring layers
		if err := m.distributionClient.WriteLightweightModel(builtModel, []string{request.Tag}); err != nil {
			http.Error(w, fmt.Sprintf("error creating lightweight model: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// If there are layer changes, we need a different approach (this shouldn't happen with context size only)
		// For now, return an error if we can't use lightweight
		http.Error(w, "only config-only changes are supported for repackaging", http.StatusBadRequest)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{
		"message": fmt.Sprintf("Successfully packaged model from %s with tag %s", request.From, request.Tag),
		"model":   request.Tag,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		m.log.Warnln("Error while encoding package response:", err)
	}
}

// handlePurge handles DELETE <inference-prefix>/models/purge requests.
func (m *Manager) handlePurge(w http.ResponseWriter, _ *http.Request) {
	if m.distributionClient == nil {
		http.Error(w, "model distribution service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := m.distributionClient.ResetStore(); err != nil {
		m.log.Warnf("Failed to purge models: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GetDiskUsage returns the disk usage of the model store.
func (m *Manager) GetDiskUsage() (int64, error, int) {
	if m.distributionClient == nil {
		return 0, errors.New("model distribution service unavailable"), http.StatusServiceUnavailable
	}

	storePath := m.distributionClient.GetStorePath()
	size, err := diskusage.Size(storePath)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %v", err), http.StatusInternalServerError
	}

	return size, nil, http.StatusOK
}

// ServeHTTP implement net/http.Handler.ServeHTTP.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	m.httpHandler.ServeHTTP(w, r)
}

// IsModelInStore checks if a given model is in the local store.
func (m *Manager) IsModelInStore(ref string) (bool, error) {
	return m.distributionClient.IsModelInStore(ref)
}

// GetModel returns a single model.
func (m *Manager) GetModel(ref string) (types.Model, error) {
	model, err := m.distributionClient.GetModel(ref)
	if err != nil {
		return nil, fmt.Errorf("error while getting model: %w", err)
	}
	return model, err
}

// GetRemoteModel returns a single remote model.
func (m *Manager) GetRemoteModel(ctx context.Context, ref string) (types.ModelArtifact, error) {
	model, err := m.registryClient.Model(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("error while getting remote model: %w", err)
	}
	return model, nil
}

// GetRemoteModelBlobURL returns the URL of a given model blob.
func (m *Manager) GetRemoteModelBlobURL(ref string, digest v1.Hash) (string, error) {
	blobURL, err := m.registryClient.BlobURL(ref, digest)
	if err != nil {
		return "", fmt.Errorf("error while getting remote model blob URL: %w", err)
	}
	return blobURL, nil
}

// BearerTokenForModel returns the bearer token needed to pull a given model.
func (m *Manager) BearerTokenForModel(ctx context.Context, ref string) (string, error) {
	tok, err := m.registryClient.BearerToken(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("error while getting bearer token for model: %w", err)
	}
	return tok, nil
}

// GetBundle returns model bundle.
func (m *Manager) GetBundle(ref string) (types.ModelBundle, error) {
	bundle, err := m.distributionClient.GetBundle(ref)
	if err != nil {
		return nil, fmt.Errorf("error while getting model bundle: %w", err)
	}
	return bundle, err
}

// PullModel pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) PullModel(model string, r *http.Request, w http.ResponseWriter) error {
	// Restrict model pull concurrency.
	select {
	case <-m.pullTokens:
	case <-r.Context().Done():
		return context.Canceled
	}
	defer func() {
		m.pullTokens <- struct{}{}
	}()

	// Set up response headers for streaming
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Check Accept header to determine content type
	acceptHeader := r.Header.Get("Accept")
	isJSON := acceptHeader == "application/json"

	if isJSON {
		w.Header().Set("Content-Type", "application/json")
	} else {
		// Defaults to text/plain
		w.Header().Set("Content-Type", "text/plain")
	}

	// Create a flusher to ensure chunks are sent immediately
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Create a progress writer that writes to the response
	progressWriter := &progressResponseWriter{
		writer:  w,
		flusher: flusher,
		isJSON:  isJSON,
	}

	// Pull the model using the Docker model distribution client
	m.log.Infoln("Pulling model:", model)
	err := m.distributionClient.PullModel(r.Context(), model, progressWriter)
	if err != nil {
		return fmt.Errorf("error while pulling model: %w", err)
	}

	return nil
}

// PushModel pushes a model from the store to the registry.
func (m *Manager) PushModel(model string, r *http.Request, w http.ResponseWriter) error {
	// Set up response headers for streaming
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Check Accept header to determine content type
	acceptHeader := r.Header.Get("Accept")
	isJSON := acceptHeader == "application/json"

	if isJSON {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}

	// Create a flusher to ensure chunks are sent immediately
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported")
	}

	// Create a progress writer that writes to the response
	progressWriter := &progressResponseWriter{
		writer:  w,
		flusher: flusher,
		isJSON:  isJSON,
	}

	// Pull the model using the Docker model distribution client
	m.log.Infoln("Pushing model:", model)
	err := m.distributionClient.PushModel(r.Context(), model, progressWriter)
	if err != nil {
		return fmt.Errorf("error while pushing model: %w", err)
	}

	return nil
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
