package distribution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/docker/model-distribution/internal/progress"
	"github.com/docker/model-distribution/internal/store"
	"github.com/docker/model-distribution/registry"
	"github.com/docker/model-distribution/tarball"
	"github.com/docker/model-distribution/types"
)

// Client provides model distribution functionality
type Client struct {
	store    *store.LocalStore
	log      *logrus.Entry
	registry *registry.Client
}

// GetStorePath returns the root path where models are stored
func (c *Client) GetStorePath() string {
	return c.store.RootPath()
}

// Option represents an option for creating a new Client
type Option func(*options)

// options holds the configuration for a new Client
type options struct {
	storeRootPath string
	logger        *logrus.Entry
	transport     http.RoundTripper
	userAgent     string
	username      string
	password      string
}

// WithStoreRootPath sets the store root path
func WithStoreRootPath(path string) Option {
	return func(o *options) {
		if path != "" {
			o.storeRootPath = path
		}
	}
}

// WithLogger sets the logger
func WithLogger(logger *logrus.Entry) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithTransport sets the HTTP transport to use when pulling and pushing models.
func WithTransport(transport http.RoundTripper) Option {
	return func(o *options) {
		if transport != nil {
			o.transport = transport
		}
	}
}

// WithUserAgent sets the User-Agent header to use when pulling and pushing models.
func WithUserAgent(ua string) Option {
	return func(o *options) {
		if ua != "" {
			o.userAgent = ua
		}
	}
}

// WithRegistryAuth sets the registry authentication credentials
func WithRegistryAuth(username, password string) Option {
	return func(o *options) {
		if username != "" && password != "" {
			o.username = username
			o.password = password
		}
	}
}

func defaultOptions() *options {
	return &options{
		logger:    logrus.NewEntry(logrus.StandardLogger()),
		transport: registry.DefaultTransport,
		userAgent: registry.DefaultUserAgent,
	}
}

// NewClient creates a new distribution client
func NewClient(opts ...Option) (*Client, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if options.storeRootPath == "" {
		return nil, fmt.Errorf("store root path is required")
	}

	s, err := store.New(store.Options{
		RootPath: options.storeRootPath,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	// Create registry client options
	registryOpts := []registry.ClientOption{
		registry.WithTransport(options.transport),
		registry.WithUserAgent(options.userAgent),
	}

	// Add auth if credentials are provided
	if options.username != "" && options.password != "" {
		registryOpts = append(registryOpts, registry.WithAuthConfig(options.username, options.password))
	}

	options.logger.Infoln("Successfully initialized store")
	return &Client{
		store:    s,
		log:      options.logger,
		registry: registry.NewClient(registryOpts...),
	}, nil
}

// PullModel pulls a model from a registry and returns the local file path
func (c *Client) PullModel(ctx context.Context, reference string, progressWriter io.Writer) error {
	c.log.Infoln("Starting model pull:", reference)

	remoteModel, err := c.registry.Model(ctx, reference)
	if err != nil {
		return fmt.Errorf("reading model from registry: %w", err)
	}

	// Check for supported type
	if err := checkCompat(remoteModel); err != nil {
		return err
	}

	// Get the remote image digest
	remoteDigest, err := remoteModel.Digest()
	if err != nil {
		c.log.Errorln("Failed to get remote image digest:", err)
		return fmt.Errorf("getting remote image digest: %w", err)
	}
	c.log.Infoln("Remote model digest:", remoteDigest.String())

	// Check if model exists in local store
	localModel, err := c.store.Read(remoteDigest.String())
	if err == nil {
		c.log.Infoln("Model found in local store:", reference)
		ggufPath, err := localModel.GGUFPath()
		if err != nil {
			return fmt.Errorf("getting gguf path: %w", err)
		}

		// Get file size for progress reporting
		fileInfo, err := os.Stat(ggufPath)
		if err != nil {
			return fmt.Errorf("getting file info: %w", err)
		}

		// Report progress for local model
		size := fileInfo.Size()
		err = progress.WriteSuccess(progressWriter, fmt.Sprintf("Using cached model: %.2f MB", float64(size)/1024/1024))
		if err != nil {
			c.log.Warnf("Writing progress: %v", err)
			// If we fail to write progress, don't try again
			progressWriter = nil
		}

		// Ensure model has the correct tag
		if err := c.store.AddTags(remoteDigest.String(), []string{reference}); err != nil {
			return fmt.Errorf("tagging model: %w", err)
		}
		return nil
	} else {
		c.log.Infoln("Model not found in local store, pulling from remote:", reference)
	}

	// Model doesn't exist in local store or digests don't match, pull from remote

	if err = c.store.Write(remoteModel, []string{reference}, progressWriter); err != nil {
		if writeErr := progress.WriteError(progressWriter, fmt.Sprintf("Error: %s", err.Error())); writeErr != nil {
			c.log.Warnf("Failed to write error message: %v", writeErr)
			// If we fail to write error message, don't try again
			progressWriter = nil
		}
		return fmt.Errorf("writing image to store: %w", err)
	}

	if err := progress.WriteSuccess(progressWriter, "Model pulled successfully"); err != nil {
		c.log.Warnf("Failed to write success message: %v", err)
		// If we fail to write success message, don't try again
		progressWriter = nil
	}

	return nil
}

// LoadModel loads the model from the reader to the store
func (c *Client) LoadModel(r io.Reader, progressWriter io.Writer) (string, error) {
	c.log.Infoln("Starting model load")

	tr := tarball.NewReader(r)
	for {
		diffID, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				c.log.Infof("Model load interrupted (likely cancelled): %v", err)
				return "", fmt.Errorf("model load interrupted: %w", err)
			}
			return "", fmt.Errorf("reading blob from stream: %w", err)
		}
		c.log.Infoln("Loading blob:", diffID)
		if err := c.store.WriteBlob(diffID, tr); err != nil {
			return "", fmt.Errorf("writing blob: %w", err)
		}
		c.log.Infoln("Loaded blob:", diffID)
	}

	manifest, digest, err := tr.Manifest()
	if err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}
	c.log.Infoln("Loading manifest:", digest.String())
	if err := c.store.WriteManifest(digest, manifest); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	c.log.Infoln("Loaded model with ID:", digest.String())

	if err := progress.WriteSuccess(progressWriter, "Model loaded successfully"); err != nil {
		c.log.Warnf("Failed to write success message: %v", err)
		// If we fail to write success message, don't try again
		progressWriter = nil
	}

	return digest.String(), nil
}

// ListModels returns all available models
func (c *Client) ListModels() ([]types.Model, error) {
	c.log.Infoln("Listing available models")
	modelInfos, err := c.store.List()
	if err != nil {
		c.log.Errorln("Failed to list models:", err)
		return nil, fmt.Errorf("listing models: %w", err)
	}

	result := make([]types.Model, 0, len(modelInfos))
	for _, modelInfo := range modelInfos {
		// Read the models
		model, err := c.store.Read(modelInfo.ID)
		if err != nil {
			c.log.Warnf("Failed to read model with ID %s: %v", modelInfo.ID, err)
			continue
		}
		result = append(result, model)
	}

	c.log.Infoln("Successfully listed models, count:", len(result))
	return result, nil
}

// GetModel returns a model by reference
func (c *Client) GetModel(reference string) (types.Model, error) {
	c.log.Infoln("Getting model by reference:", reference)
	model, err := c.store.Read(reference)
	if err != nil {
		c.log.Errorln("Failed to get model:", err, "reference:", reference)
		return nil, fmt.Errorf("get model '%q': %w", reference, err)
	}

	return model, nil
}

// IsModelInStore checks if a model with the given reference is in the local store
func (c *Client) IsModelInStore(reference string) (bool, error) {
	c.log.Infoln("Checking model by reference:", reference)
	if _, err := c.store.Read(reference); errors.Is(err, ErrModelNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

type DeleteModelAction struct {
	Untagged *string `json:"Untagged,omitempty"`
	Deleted  *string `json:"Deleted,omitempty"`
}

type DeleteModelResponse []DeleteModelAction

// DeleteModel deletes a model
func (c *Client) DeleteModel(reference string, force bool) (*DeleteModelResponse, error) {
	mdl, err := c.store.Read(reference)
	if err != nil {
		return &DeleteModelResponse{}, err
	}
	id, err := mdl.ID()
	if err != nil {
		return &DeleteModelResponse{}, fmt.Errorf("getting model ID: %w", err)
	}
	isTag := id != reference

	resp := DeleteModelResponse{}

	if isTag {
		c.log.Infoln("Untagging model:", reference)
		tags, err := c.store.RemoveTags([]string{reference})
		if err != nil {
			c.log.Errorln("Failed to untag model:", err, "tag:", reference)
			return &DeleteModelResponse{}, fmt.Errorf("untagging model: %w", err)
		}
		for _, t := range tags {
			resp = append(resp, DeleteModelAction{Untagged: &t})
		}
		if len(mdl.Tags()) > 1 {
			return &resp, nil
		}
	}

	if len(mdl.Tags()) > 1 && !force {
		// if the reference is not a tag and there are multiple tags, return an error unless forced
		return &DeleteModelResponse{}, fmt.Errorf(
			"unable to delete %q (must be forced) due to multiple tag references: %w",
			reference, ErrConflict,
		)
	}

	c.log.Infoln("Deleting model:", id)
	deletedID, tags, err := c.store.Delete(id)
	if err != nil {
		c.log.Errorln("Failed to delete model:", err, "tag:", reference)
		return &DeleteModelResponse{}, fmt.Errorf("deleting model: %w", err)
	}
	c.log.Infoln("Successfully deleted model:", reference)
	for _, t := range tags {
		resp = append(resp, DeleteModelAction{Untagged: &t})
	}
	resp = append(resp, DeleteModelAction{Deleted: &deletedID})
	return &resp, nil
}

// Tag adds a tag to a model
func (c *Client) Tag(source string, target string) error {
	c.log.Infoln("Tagging model, source:", source, "target:", target)
	return c.store.AddTags(source, []string{target})
}

// PushModel pushes a tagged model from the content store to the registry.
func (c *Client) PushModel(ctx context.Context, tag string, progressWriter io.Writer) (err error) {
	// Parse the tag
	target, err := c.registry.NewTarget(tag)
	if err != nil {
		return fmt.Errorf("new tag: %w", err)
	}

	// Get the model from the store
	mdl, err := c.store.Read(tag)
	if err != nil {
		return fmt.Errorf("reading model: %w", err)
	}

	// Push the model
	c.log.Infoln("Pushing model:", tag)
	if err := target.Write(ctx, mdl, progressWriter); err != nil {
		c.log.Errorln("Failed to push image:", err, "reference:", tag)
		if writeErr := progress.WriteError(progressWriter, fmt.Sprintf("Error: %s", err.Error())); writeErr != nil {
			c.log.Warnf("Failed to write error message: %v", writeErr)
		}
		return fmt.Errorf("pushing image: %w", err)
	}

	c.log.Infoln("Successfully pushed model:", tag)
	if err := progress.WriteSuccess(progressWriter, "Model pushed successfully"); err != nil {
		c.log.Warnf("Failed to write success message: %v", err)
	}

	return nil
}

func (c *Client) ResetStore() error {
	c.log.Infoln("Resetting store")
	if err := c.store.Reset(); err != nil {
		c.log.Errorln("Failed to reset store:", err)
		return fmt.Errorf("resetting store: %w", err)
	}
	return nil
}

func checkCompat(image types.ModelArtifact) error {
	manifest, err := image.Manifest()
	if err != nil {
		return err
	}
	if manifest.Config.MediaType != types.MediaTypeModelConfigV01 {
		return fmt.Errorf("config type %q is unsupported: %w", manifest.Config.MediaType, ErrUnsupportedMediaType)
	}
	return nil
}
