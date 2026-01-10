package distribution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/huggingface"
	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/internal/store"
	"github.com/docker/model-runner/pkg/distribution/oci/authn"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/tarball"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/sirupsen/logrus"
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
	plainHTTP     bool
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

// WithPlainHTTP allows connecting to registries using plain HTTP instead of HTTPS.
func WithPlainHTTP(plain bool) Option {
	return func(o *options) {
		o.plainHTTP = plain
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
		registry.WithPlainHTTP(options.plainHTTP),
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

// normalizeModelName adds the default organization prefix (ai/) and tag (:latest) if missing.
// It also resolves IDs to full IDs.
// This is a private method used internally by the Client.
func (c *Client) normalizeModelName(model string) string {
	const (
		defaultOrg = "ai"
		defaultTag = "latest"
	)

	model = strings.TrimSpace(model)
	if model == "" {
		return model
	}

	// If it looks like an ID or digest, try to resolve it to full ID
	if c.looksLikeID(model) || c.looksLikeDigest(model) {
		if fullID := c.resolveID(model); fullID != "" {
			return fullID
		}
		return model
	}

	// Split name vs tag, where ':' is a tag separator only if it's after the last '/'
	lastSlash := strings.LastIndex(model, "/")
	lastColon := strings.LastIndex(model, ":")

	name := model
	tag := defaultTag
	hasTag := lastColon > lastSlash

	if hasTag {
		name = model[:lastColon]
		// Preserve tag as-is; if empty, fall back to defaultTag
		if t := model[lastColon+1:]; t != "" {
			tag = t
		}
	}

	// If name has no registry (domain with dot before first slash), apply default org if missing slash
	firstSlash := strings.Index(name, "/")
	hasRegistry := firstSlash > 0 && strings.Contains(name[:firstSlash], ".")

	if !hasRegistry && !strings.Contains(name, "/") {
		name = defaultOrg + "/" + name
	}

	// Lowercase ONLY the name part (registry/org/repo). Tag stays unchanged.
	name = strings.ToLower(name)

	return name + ":" + tag
}

// looksLikeID returns true for short & long hex IDs (12 or 64 chars)
func (c *Client) looksLikeID(s string) bool {
	n := len(s)
	if n != 12 && n != 64 {
		return false
	}
	for i := 0; i < n; i++ {
		ch := s[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// looksLikeDigest returns true for e.g. "sha256:<64-hex>"
func (c *Client) looksLikeDigest(s string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	hashPart := s[len(prefix):]
	// SHA256 digests must be exactly 64 hex characters
	if len(hashPart) != 64 {
		return false
	}
	for i := 0; i < 64; i++ {
		ch := hashPart[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// resolveID attempts to resolve a short ID or digest to a full model ID
// by checking all models in the store. Returns empty string if not found.
func (c *Client) resolveID(id string) string {
	models, err := c.ListModels()
	if err != nil {
		return ""
	}

	for _, m := range models {
		fullID, err := m.ID()
		if err != nil {
			continue
		}

		// Check short ID (12 chars) - match against the hex part after "sha256:"
		if len(id) == 12 && strings.HasPrefix(fullID, "sha256:") {
			if len(fullID) >= 19 && fullID[7:19] == id {
				return fullID
			}
		}

		// Check full ID match (with or without sha256: prefix)
		if fullID == id || strings.TrimPrefix(fullID, "sha256:") == id {
			return fullID
		}
	}

	return ""
}

// PullModel pulls a model from a registry and returns the local file path
func (c *Client) PullModel(ctx context.Context, reference string, progressWriter io.Writer, bearerToken ...string) error {
	// Store original reference before normalization (needed for case-sensitive HuggingFace API)
	originalReference := reference
	// Normalize the model reference
	reference = c.normalizeModelName(reference)
	c.log.Infoln("Starting model pull:", utils.SanitizeForLog(reference))

	// Handle bearer token for registry authentication
	var token string
	if len(bearerToken) > 0 && bearerToken[0] != "" {
		token = bearerToken[0]
	}

	// HuggingFace references always use native pull (download raw files from HF Hub)
	if isHuggingFaceReference(originalReference) {
		c.log.Infoln("Using native HuggingFace pull for:", utils.SanitizeForLog(reference))
		// Pass original reference to preserve case-sensitivity for HuggingFace API
		return c.pullNativeHuggingFace(ctx, originalReference, progressWriter, token)
	}

	// For non-HF references, use OCI registry
	registryClient := c.registry
	if token != "" {
		// Create a temporary registry client with bearer token authentication
		auth := authn.NewBearer(token)
		registryClient = registry.FromClient(c.registry, registry.WithAuth(auth))
	}

	// Fetch the remote model to get the manifest
	remoteModel, err := registryClient.Model(ctx, reference)
	if err != nil {
		// Check if the error should be converted to registry.ErrModelNotFound for API compatibility
		// If the error already matches ErrModelNotFound, return it directly to preserve errors.Is compatibility
		if errors.Is(err, registry.ErrModelNotFound) {
			return err
		}
		return fmt.Errorf("reading model from registry: %w", err)
	}

	// Get the remote image digest immediately to ensure we work with a consistent manifest
	// This prevents race conditions if the tag is updated during the pull
	remoteDigest, err := remoteModel.Digest()
	if err != nil {
		c.log.Errorln("Failed to get remote image digest:", err)
		return fmt.Errorf("getting remote image digest: %w", err)
	}
	c.log.Infoln("Remote model digest:", remoteDigest.String())

	// Check for incomplete downloads and prepare resume offsets
	layers, err := remoteModel.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	// Build a map of digest -> resume offset for layers with incomplete downloads
	resumeOffsets := make(map[string]int64)
	for _, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			c.log.Warnf("Failed to get layer digest: %v", err)
			continue
		}

		// Check if there's an incomplete download for this layer (use DiffID for uncompressed models)
		diffID, err := layer.DiffID()
		if err != nil {
			c.log.Warnf("Failed to get layer diffID: %v", err)
			continue
		}

		incompleteSize, err := c.store.GetIncompleteSize(diffID)
		if err != nil {
			c.log.Warnf("Failed to check incomplete size for layer %s: %v", digest, err)
			continue
		}

		if incompleteSize > 0 {
			c.log.Infof("Found incomplete download for layer %s: %d bytes", digest, incompleteSize)
			resumeOffsets[digest.String()] = incompleteSize
		}
	}

	// If we have any incomplete downloads, create a new context with resume offsets
	// and re-fetch using the original reference to ensure compatibility with all registries
	var rangeSuccess *remote.RangeSuccess
	if len(resumeOffsets) > 0 {
		c.log.Infof("Resuming %d interrupted layer download(s)", len(resumeOffsets))
		// Create a RangeSuccess tracker to record which Range requests succeed
		rangeSuccess = &remote.RangeSuccess{}
		ctx = remote.WithResumeOffsets(ctx, resumeOffsets)
		ctx = remote.WithRangeSuccess(ctx, rangeSuccess)
		// Re-fetch the model using the original tag reference
		// The digest has already been validated above, and the resume context will handle layer resumption
		c.log.Infof("Re-fetching model with original reference for resume: %s", utils.SanitizeForLog(reference))
		remoteModel, err = registryClient.Model(ctx, reference)
		if err != nil {
			return fmt.Errorf("reading model from registry with resume context: %w", err)
		}
	}

	// Check for supported type
	if err := checkCompat(remoteModel, c.log, reference, progressWriter); err != nil {
		return err
	}

	// Check if model exists in local store
	localModel, err := c.store.Read(remoteDigest.String())
	if err == nil {
		c.log.Infoln("Model found in local store:", utils.SanitizeForLog(reference))
		cfg, err := localModel.Config()
		if err != nil {
			return fmt.Errorf("getting cached model config: %w", err)
		}

		err = progress.WriteSuccess(progressWriter, fmt.Sprintf("Using cached model: %s", cfg.GetSize()))
		if err != nil {
			c.log.Warnf("Writing progress: %v", err)
		}

		// Ensure model has the correct tag
		if err := c.store.AddTags(remoteDigest.String(), []string{reference}); err != nil {
			return fmt.Errorf("tagging model: %w", err)
		}
		return nil
	} else {
		c.log.Infoln("Model not found in local store, pulling from remote:", utils.SanitizeForLog(reference))
	}

	// Model doesn't exist in local store or digests don't match, pull from remote

	// Pass rangeSuccess to store.Write for resume detection
	var writeOpts []store.WriteOption
	if rangeSuccess != nil {
		writeOpts = append(writeOpts, store.WithRangeSuccess(rangeSuccess))
	}
	if err = c.store.Write(remoteModel, []string{reference}, progressWriter, writeOpts...); err != nil {
		if writeErr := progress.WriteError(progressWriter, fmt.Sprintf("Error: %s", err.Error())); writeErr != nil {
			c.log.Warnf("Failed to write error message: %v", writeErr)
		}
		return fmt.Errorf("writing image to store: %w", err)
	}

	if err := progress.WriteSuccess(progressWriter, "Model pulled successfully"); err != nil {
		c.log.Warnf("Failed to write success message: %v", err)
	}

	return nil
}

// LoadModel loads the model from the reader to the store
func (c *Client) LoadModel(r io.Reader, progressWriter io.Writer) (string, error) {
	c.log.Infoln("Starting model load")

	tr := tarball.NewReader(r)
	for {
		diffID, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				c.log.Infof("Model load interrupted (likely cancelled): %s", utils.SanitizeForLog(err.Error()))
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
	c.log.Infoln("Getting model by reference:", utils.SanitizeForLog(reference))
	normalizedRef := c.normalizeModelName(reference)
	model, err := c.store.Read(normalizedRef)
	if err != nil {
		c.log.Errorln("Failed to get model:", err, "reference:", utils.SanitizeForLog(reference))
		return nil, fmt.Errorf("get model '%q': %w", utils.SanitizeForLog(reference), err)
	}

	return model, nil
}

// IsModelInStore checks if a model with the given reference is in the local store
func (c *Client) IsModelInStore(reference string) (bool, error) {
	c.log.Infoln("Checking model by reference:", utils.SanitizeForLog(reference))
	normalizedRef := c.normalizeModelName(reference)
	if _, err := c.store.Read(normalizedRef); errors.Is(err, ErrModelNotFound) {
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
	normalizedRef := c.normalizeModelName(reference)
	mdl, err := c.store.Read(normalizedRef)
	if err != nil {
		return &DeleteModelResponse{}, err
	}
	id, err := mdl.ID()
	if err != nil {
		return &DeleteModelResponse{}, fmt.Errorf("getting model ID: %w", err)
	}

	// Check if this is a digest reference (contains @)
	// Digest references like "name@sha256:..." should be treated as ID references, not tags
	isDigestReference := strings.Contains(reference, "@")
	isTag := id != normalizedRef && !isDigestReference

	resp := DeleteModelResponse{}

	if isTag {
		c.log.Infoln("Untagging model:", reference)
		tags, err := c.store.RemoveTags([]string{normalizedRef})
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
	c.log.Infoln("Tagging model, source:", source, "target:", utils.SanitizeForLog(target))
	normalizedSource := c.normalizeModelName(source)
	normalizedTarget := c.normalizeModelName(target)
	return c.store.AddTags(normalizedSource, []string{normalizedTarget})
}

// PushModel pushes a tagged model from the content store to the registry.
func (c *Client) PushModel(ctx context.Context, tag string, progressWriter io.Writer) (err error) {
	// Parse the tag
	target, err := c.registry.NewTarget(tag)
	if err != nil {
		return fmt.Errorf("new tag: %w", err)
	}

	// Get the model from the store
	normalizedRef := c.normalizeModelName(tag)
	mdl, err := c.store.Read(normalizedRef)
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

// WriteLightweightModel writes a model to the store without transferring layer data.
// This is used for config-only modifications where the layer data hasn't changed.
// The layers must already exist in the store.
func (c *Client) WriteLightweightModel(mdl types.ModelArtifact, tags []string) error {
	c.log.Infoln("Writing lightweight model variant")
	normalizedTags := make([]string, len(tags))
	for i, tag := range tags {
		normalizedTags[i] = c.normalizeModelName(tag)
	}
	return c.store.WriteLightweight(mdl, normalizedTags)
}

func (c *Client) ResetStore() error {
	c.log.Infoln("Resetting store")
	if err := c.store.Reset(); err != nil {
		c.log.Errorln("Failed to reset store:", err)
		return fmt.Errorf("resetting store: %w", err)
	}
	return nil
}

// GetBundle returns a types.Bundle containing the model, creating one as necessary
func (c *Client) GetBundle(ref string) (types.ModelBundle, error) {
	normalizedRef := c.normalizeModelName(ref)
	return c.store.BundleForModel(normalizedRef)
}

func GetSupportedFormats() []types.Format {
	if platform.SupportsVLLM() {
		return []types.Format{types.FormatGGUF, types.FormatSafetensors}
	}
	return []types.Format{types.FormatGGUF}
}

func checkCompat(image types.ModelArtifact, log *logrus.Entry, reference string, progressWriter io.Writer) error {
	manifest, err := image.Manifest()
	if err != nil {
		return err
	}
	if manifest.Config.MediaType != types.MediaTypeModelConfigV01 {
		return fmt.Errorf("config type %q is unsupported: %w", manifest.Config.MediaType, ErrUnsupportedMediaType)
	}

	// Check if the model format is supported
	config, err := image.Config()
	if err != nil {
		return fmt.Errorf("reading model config: %w", err)
	}

	if config.GetFormat() == "" {
		log.Warnf("Model format field is empty for %s, unable to verify format compatibility",
			utils.SanitizeForLog(reference))
	} else if !slices.Contains(GetSupportedFormats(), config.GetFormat()) {
		// Write warning but continue with pull
		log.Warnln(warnUnsupportedFormat)
		if err := progress.WriteWarning(progressWriter, warnUnsupportedFormat); err != nil {
			log.Warnf("Failed to write warning message: %v", err)
		}
		// Don't return an error - allow the pull to continue
	}

	return nil
}

// isHuggingFaceReference checks if a reference is a HuggingFace model reference
func isHuggingFaceReference(reference string) bool {
	return strings.HasPrefix(reference, "huggingface.co/") ||
		strings.HasPrefix(reference, "hf.co/")
}

// parseHFReference extracts repo, revision, and tag from a HF reference
// e.g., "huggingface.co/org/model:revision" -> ("org/model", "main", "revision")
// e.g., "hf.co/org/model:latest" -> ("org/model", "main", "latest")
// e.g., "hf.co/org/model:Q4_K_M" -> ("org/model", "main", "Q4_K_M")
// The tag is used for GGUF quantization selection, while revision is always "main" for HuggingFace
func parseHFReference(reference string) (repo, revision, tag string) {
	// Remove registry prefix (handle both hf.co and huggingface.co)
	ref := strings.TrimPrefix(reference, "huggingface.co/")
	ref = strings.TrimPrefix(ref, "hf.co/")

	// Split by colon to get tag
	parts := strings.SplitN(ref, ":", 2)
	repo = parts[0]

	// Default tag is "latest"
	tag = "latest"
	if len(parts) == 2 && parts[1] != "" {
		tag = parts[1]
	}

	// Revision is always "main" for HuggingFace repos
	// (the tag is used for quantization selection, not git revision)
	revision = "main"

	return repo, revision, tag
}

// pullNativeHuggingFace pulls a native HuggingFace repository (non-OCI format)
// This is used when the model is stored as raw files (safetensors) on HuggingFace Hub
func (c *Client) pullNativeHuggingFace(ctx context.Context, reference string, progressWriter io.Writer, token string) error {
	repo, revision, tag := parseHFReference(reference)
	c.log.Infof("Pulling native HuggingFace model: repo=%s, revision=%s, tag=%s", utils.SanitizeForLog(repo), utils.SanitizeForLog(revision), utils.SanitizeForLog(tag))

	// Create HuggingFace client
	hfOpts := []huggingface.ClientOption{
		huggingface.WithUserAgent(registry.DefaultUserAgent),
	}
	if token != "" {
		hfOpts = append(hfOpts, huggingface.WithToken(token))
	}
	hfClient := huggingface.NewClient(hfOpts...)

	// Create temp directory for downloads
	tempDir, err := os.MkdirTemp("", "hf-model-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Build model from HuggingFace repository
	// The tag is used for GGUF quantization selection (e.g., "Q4_K_M", "Q8_0")
	model, err := huggingface.BuildModel(ctx, hfClient, repo, revision, tag, tempDir, progressWriter)
	if err != nil {
		// Convert HuggingFace errors to registry errors for consistent handling
		var authErr *huggingface.AuthError
		var notFoundErr *huggingface.NotFoundError
		if errors.As(err, &authErr) {
			return registry.ErrUnauthorized
		}
		if errors.As(err, &notFoundErr) {
			return registry.ErrModelNotFound
		}
		if writeErr := progress.WriteError(progressWriter, fmt.Sprintf("Error: %s", err.Error())); writeErr != nil {
			c.log.Warnf("Failed to write error message: %v", writeErr)
		}
		return fmt.Errorf("build model from HuggingFace: %w", err)
	}

	// Write model to store with normalized tag
	storageTag := c.normalizeModelName(reference)
	c.log.Infof("Writing model to store with tag: %s", utils.SanitizeForLog(storageTag))
	if err := c.store.Write(model, []string{storageTag}, progressWriter); err != nil {
		if writeErr := progress.WriteError(progressWriter, fmt.Sprintf("Error: %s", err.Error())); writeErr != nil {
			c.log.Warnf("Failed to write error message: %v", writeErr)
		}
		return fmt.Errorf("writing model to store: %w", err)
	}

	if err := progress.WriteSuccess(progressWriter, "Model pulled successfully"); err != nil {
		c.log.Warnf("Failed to write success message: %v", err)
	}

	return nil
}
