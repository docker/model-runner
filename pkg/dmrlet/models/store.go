// Package models provides model store integration for dmrlet.
package models

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/sirupsen/logrus"
)

// Store wraps the distribution client to provide model management for dmrlet.
type Store struct {
	client *distribution.Client
	log    *logrus.Entry
}

// StoreOption configures the Store.
type StoreOption func(*storeOptions)

type storeOptions struct {
	rootPath string
	logger   *logrus.Entry
}

// WithRootPath sets the model store root path.
func WithRootPath(path string) StoreOption {
	return func(o *storeOptions) {
		o.rootPath = path
	}
}

// WithLogger sets the logger for the store.
func WithLogger(logger *logrus.Entry) StoreOption {
	return func(o *storeOptions) {
		o.logger = logger
	}
}

// DefaultStorePath returns the default model store path.
func DefaultStorePath() string {
	if path := os.Getenv("DMRLET_MODEL_STORE"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".docker", "models")
	}
	return filepath.Join(home, ".docker", "models")
}

// NewStore creates a new model store.
func NewStore(opts ...StoreOption) (*Store, error) {
	options := &storeOptions{
		rootPath: DefaultStorePath(),
		logger:   logrus.NewEntry(logrus.StandardLogger()),
	}
	for _, opt := range opts {
		opt(options)
	}

	// Create the store directory if it doesn't exist
	if err := os.MkdirAll(options.rootPath, 0755); err != nil {
		return nil, fmt.Errorf("creating store directory: %w", err)
	}

	client, err := distribution.NewClient(
		distribution.WithStoreRootPath(options.rootPath),
		distribution.WithLogger(options.logger),
	)
	if err != nil {
		return nil, fmt.Errorf("creating distribution client: %w", err)
	}

	return &Store{
		client: client,
		log:    options.logger,
	}, nil
}

// EnsureModel ensures a model is available locally, pulling it if necessary.
func (s *Store) EnsureModel(ctx context.Context, ref string, progressWriter io.Writer) error {
	s.log.Infof("Ensuring model is available: %s", ref)

	// Check if model already exists
	inStore, err := s.client.IsModelInStore(ref)
	if err != nil {
		return fmt.Errorf("checking model in store: %w", err)
	}

	if inStore {
		s.log.Infof("Model already exists: %s", ref)
		return nil
	}

	// Pull the model
	s.log.Infof("Pulling model: %s", ref)
	if err := s.client.PullModel(ctx, ref, progressWriter); err != nil {
		return fmt.Errorf("pulling model: %w", err)
	}

	return nil
}

// GetBundle returns the model bundle for a given reference.
func (s *Store) GetBundle(ref string) (types.ModelBundle, error) {
	bundle, err := s.client.GetBundle(ref)
	if err != nil {
		return nil, fmt.Errorf("getting bundle for %s: %w", ref, err)
	}
	return bundle, nil
}

// GetModel returns a model by reference.
func (s *Store) GetModel(ref string) (types.Model, error) {
	model, err := s.client.GetModel(ref)
	if err != nil {
		return nil, fmt.Errorf("getting model %s: %w", ref, err)
	}
	return model, nil
}

// ListModels returns all available models.
func (s *Store) ListModels() ([]types.Model, error) {
	return s.client.ListModels()
}

// GetStorePath returns the root path of the model store.
func (s *Store) GetStorePath() string {
	return s.client.GetStorePath()
}
