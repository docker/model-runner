package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// maximumConcurrentModelPulls is the maximum number of concurrent model
	// pulls that a model manager will allow.
	maximumConcurrentModelPulls = 2
)

// Manager handles the business logic for model management operations.
type Manager struct {
	// log is the associated logger.
	log logging.Logger
	// distributionClient is the client for model distribution.
	distributionClient *distribution.Client
	// registryClient is the client for model registry.
	registryClient *registry.Client
	// pullTokens is a semaphore used to restrict the maximum number of
	// concurrent pull requests.
	pullTokens chan struct{}
}

// NewManager creates a new model models with the provided clients.
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

	// Create the model registry client.
	registryClient := registry.NewClient(
		registry.WithTransport(c.Transport),
		registry.WithUserAgent(c.UserAgent),
	)

	tokens := make(chan struct{}, maximumConcurrentModelPulls)

	// Populate the pull concurrency semaphore.
	for i := 0; i < maximumConcurrentModelPulls; i++ {
		tokens <- struct{}{}
	}

	return &Manager{
		log:                log,
		distributionClient: distributionClient,
		registryClient:     registryClient,
		pullTokens:         tokens,
	}
}

// GetLocal returns a single model by reference.
// This is the core business logic for retrieving a model from the distribution client.
func (m *Manager) GetLocal(ref string) (types.Model, error) {
	if m.distributionClient == nil {
		return nil, fmt.Errorf("model distribution service unavailable")
	}

	// Query the model - first try without normalization (as ID), then with normalization
	model, err := m.distributionClient.GetModel(ref)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// If not found as-is, try with normalization
		normalizedRef := NormalizeModelName(ref)
		if normalizedRef != ref { // only try normalized if it's different
			model, err = m.distributionClient.GetModel(normalizedRef)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("error while getting model: %w", err)
	}
	return model, nil
}

// ResolveID resolves a model reference to a model ID. If resolution fails, it returns the original ref.
func (m *Manager) ResolveID(modelRef string) string {
	// Sanitize modelRef to prevent log forgery
	sanitizedModelRef := utils.SanitizeForLog(modelRef, -1)
	model, err := m.GetLocal(sanitizedModelRef)
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

func (m *Manager) GetDiskUsage() (int64, error) {
	if m.distributionClient == nil {
		return 0, errors.New("model distribution service unavailable")
	}
	storePath := m.distributionClient.GetStorePath()
	size, err := diskusage.Size(storePath)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %w", err)
	}
	return size, nil
}

// GetRemote returns a single remote model.
func (m *Manager) GetRemote(ctx context.Context, ref string) (types.ModelArtifact, error) {
	if m.registryClient == nil {
		return nil, fmt.Errorf("model registry service unavailable")
	}
	normalizedRef := NormalizeModelName(ref)
	model, err := m.registryClient.Model(ctx, normalizedRef)
	if err != nil {
		return nil, fmt.Errorf("error while getting remote model: %w", err)
	}
	return model, nil
}

// GetRemoteBlobURL returns the URL of a given model blob.
func (m *Manager) GetRemoteBlobURL(ref string, digest v1.Hash) (string, error) {
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
	return bundle, nil
}

// InStore checks if a given model is in the local store.
func (m *Manager) InStore(ref string) (bool, error) {
	return m.distributionClient.IsModelInStore(ref)
}

// List returns all models.
func (m *Manager) List() ([]*Model, error) {
	models, err := m.RawList()
	if err != nil {
		return nil, err
	}

	apiModels := make([]*Model, 0, len(models))
	for _, model := range models {
		apiModel, err := ToModel(model)
		if err != nil {
			m.log.Warnf("error while converting model, skipping: %v", err)
			continue
		}
		apiModels = append(apiModels, apiModel)
	}

	return apiModels, nil
}

func (m *Manager) RawList() ([]types.Model, error) {
	if m.distributionClient == nil {
		return nil, fmt.Errorf("model distribution models unavailable")
	}
	models, err := m.distributionClient.ListModels()
	if err != nil {
		return nil, fmt.Errorf("error while listing models: %w", err)
	}
	return models, nil
}

// Delete deletes a model from storage and returns the delete response
func (m *Manager) Delete(reference string, force bool) (*distribution.DeleteModelResponse, error) {
	if m.distributionClient == nil {
		return nil, errors.New("model distribution service unavailable")
	}

	resp, err := m.distributionClient.DeleteModel(reference, force)
	if err != nil {
		return nil, fmt.Errorf("error while deleting model: %w", err)
	}
	return resp, nil
}

// Pull pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) Pull(model string, bearerToken string, r *http.Request, w http.ResponseWriter) error {
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
	m.log.Infoln("Pulling model:", utils.SanitizeForLog(model, -1))

	// Use bearer token if provided
	var err error
	if bearerToken != "" {
		m.log.Infoln("Using provided bearer token for authentication")
		err = m.distributionClient.PullModel(r.Context(), model, progressWriter, bearerToken)
	} else {
		err = m.distributionClient.PullModel(r.Context(), model, progressWriter)
	}

	if err != nil {
		return fmt.Errorf("error while pulling model: %w", err)
	}

	return nil
}

func (m *Manager) Load(r io.Reader, progressWriter io.Writer) error {
	if m.distributionClient == nil {
		return fmt.Errorf("model distribution service unavailable")
	}
	_, err := m.distributionClient.LoadModel(r, progressWriter)
	if err != nil {
		return fmt.Errorf("error while loading model: %w", err)
	}
	return nil
}

func (m *Manager) Tag(ref, target string) error {
	if m.distributionClient == nil {
		return fmt.Errorf("model distribution service unavailable")
	}

	// First try to tag using the provided model reference as-is
	err := m.distributionClient.Tag(ref, target)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// Check if the model parameter is a model ID (starts with sha256:) or is a partial name
		var foundModelRef string
		found := false

		// If it looks like an ID, try to find the model by ID
		if strings.HasPrefix(ref, "sha256:") || len(ref) == 12 { // 12-char short ID
			// Get all models and find the one matching this ID
			models, listErr := m.distributionClient.ListModels()
			if listErr != nil {
				return fmt.Errorf("error listing models: %w", listErr)
			}

			for _, mModel := range models {
				modelID, idErr := mModel.ID()
				if idErr != nil {
					m.log.Warnf("Failed to get model ID: %v", idErr)
					continue
				}

				// Check if the model ID matches (can be full or short ID)
				if modelID == ref || strings.HasPrefix(modelID, ref) {
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
				return fmt.Errorf("error listing models: %w", listErr)
			}

			// Look for a model whose tags match the provided reference
			for _, model := range models {
				for _, tagStr := range model.Tags() {
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
					if namePart == ref {
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
			return distribution.ErrModelNotFound
		}

		// Now tag using the found model reference (the matching tag)
		if tagErr := m.distributionClient.Tag(foundModelRef, target); tagErr != nil {
			m.log.Warnf("Failed to apply tag %q to resolved model %q: %v", target, foundModelRef, tagErr)
			return fmt.Errorf("error while tagging model: %w", tagErr)
		}
	} else if err != nil {
		return fmt.Errorf("error while tagging model: %w", err)
	}
	return nil
}

// Push pushes a model from the store to the registry.
func (m *Manager) Push(model string, r *http.Request, w http.ResponseWriter) error {
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

func (m *Manager) Package(ref string, tag string, contextSize *int32) error {
	// Create a builder from an existing model by getting the bundle first
	// Since ModelArtifact interface is needed to work with the builder
	bundle, err := m.distributionClient.GetBundle(ref)
	if err != nil {
		return fmt.Errorf("error while getting model bundle: %w", err)
	}

	// Create a builder from the existing model artifact (from the bundle)
	modelArtifact, ok := bundle.(types.ModelArtifact)
	if !ok {
		return fmt.Errorf("model bundle is not a valid model artifact")
	}

	// Create a builder from the existing model
	bldr, err := builder.FromModel(modelArtifact)
	if err != nil {
		return fmt.Errorf("error while building model bundle: %w", err)
	}

	// Apply context size if specified
	if contextSize != nil {
		bldr = bldr.WithContextSize(*contextSize)
	}

	// Get the built model artifact
	builtModel := bldr.Model()

	// Check if we can use lightweight repackaging (config-only changes from existing model)
	useLightweight := bldr.HasOnlyConfigChanges()

	if useLightweight {
		// Use the lightweight method to avoid re-transferring layers
		if err := m.distributionClient.WriteLightweightModel(builtModel, []string{tag}); err != nil {
			return fmt.Errorf("error writing model: %w", err)
		}
	} else {
		return err
	}
	return nil
}

func (m *Manager) Purge() error {
	if m.distributionClient == nil {
		return fmt.Errorf("model distribution service unavailable")
	}
	if err := m.distributionClient.ResetStore(); err != nil {
		m.log.Warnf("Failed to purge models: %v", err)
		return fmt.Errorf("error while purging models: %w", err)
	}
	return nil
}
