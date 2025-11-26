package models

import (
	"errors"
	"fmt"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
)

// Service handles the business logic for model management operations.
// It is separate from HTTP handling concerns and can be used by multiple
// interfaces (HTTP, CLI, gRPC, etc.).
type Service struct {
	// log is the associated logger.
	log logging.Logger
	// distributionClient is the client for model distribution.
	distributionClient *distribution.Client
	// registryClient is the client for model registry.
	registryClient *registry.Client
}

// NewService creates a new model service with the provided clients.
func NewService(log logging.Logger, c ClientConfig) *Service {
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
	return &Service{
		log:                log,
		distributionClient: distributionClient,
		registryClient:     registryClient,
	}
}

// GetModel returns a single model by reference.
// This is the core business logic for retrieving a model from the distribution client.
func (s *Service) GetModel(ref string) (types.Model, error) {
	if s.distributionClient == nil {
		return nil, fmt.Errorf("model distribution service unavailable")
	}

	// Query the model - first try without normalization (as ID), then with normalization
	model, err := s.distributionClient.GetModel(ref)
	if err != nil && errors.Is(err, distribution.ErrModelNotFound) {
		// If not found as-is, try with normalization
		normalizedRef := NormalizeModelName(ref)
		if normalizedRef != ref { // only try normalized if it's different
			model, err = s.distributionClient.GetModel(normalizedRef)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("error while getting model: %w", err)
	}
	return model, nil
}

// ResolveModelID resolves a model reference to a model ID. If resolution fails, it returns the original ref.
func (s *Service) ResolveModelID(modelRef string) string {
	// Sanitize modelRef to prevent log forgery
	sanitizedModelRef := utils.SanitizeForLog(modelRef, -1)

	model, err := s.GetModel(sanitizedModelRef)
	if err != nil {
		s.log.Warnf("Failed to resolve model ref %s to ID: %v", sanitizedModelRef, err)
		return sanitizedModelRef
	}

	modelID, err := model.ID()
	if err != nil {
		s.log.Warnf("Failed to get model ID for ref %s: %v", sanitizedModelRef, err)
		return sanitizedModelRef
	}

	return modelID
}

func (s *Service) GetDiskUsage() (int64, error) {
	if s.distributionClient == nil {
		return 0, errors.New("model distribution service unavailable")
	}

	storePath := s.distributionClient.GetStorePath()
	size, err := diskusage.Size(storePath)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %w", err)
	}

	return size, nil
}
