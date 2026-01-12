// Package remote provides registry operations using containerd's remotes.
// This replaces go-containerregistry's remote package.
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/errdefs"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/authn"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	godigest "github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// DefaultTransport is the default HTTP transport used for registry operations.
	DefaultTransport = http.DefaultTransport
)

// Option configures remote operations.
type Option func(*options)

type options struct {
	ctx       context.Context
	transport http.RoundTripper
	userAgent string
	auth      authn.Authenticator
	keychain  authn.Keychain
	progress  chan<- oci.Update
	plainHTTP bool
}

// WithContext sets the context for remote operations.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// WithTransport sets the HTTP transport.
func WithTransport(t http.RoundTripper) Option {
	return func(o *options) {
		o.transport = t
	}
}

// WithUserAgent sets the user agent header.
func WithUserAgent(ua string) Option {
	return func(o *options) {
		o.userAgent = ua
	}
}

// WithAuth sets the authenticator.
func WithAuth(auth authn.Authenticator) Option {
	return func(o *options) {
		o.auth = auth
	}
}

// WithAuthFromKeychain sets authentication from a keychain.
func WithAuthFromKeychain(kc authn.Keychain) Option {
	return func(o *options) {
		o.keychain = kc
	}
}

// WithProgress sets a channel for receiving progress updates.
func WithProgress(ch chan<- oci.Update) Option {
	return func(o *options) {
		o.progress = ch
	}
}

// WithPlainHTTP allows connecting to registries using plain HTTP instead of HTTPS.
func WithPlainHTTP(plain bool) Option {
	return func(o *options) {
		o.plainHTTP = plain
	}
}

// WithResumeOffsets is a context key for storing resume offsets.
type resumeOffsetsKey struct{}

// WithResumeOffsets adds resume offsets to a context.
func WithResumeOffsets(ctx context.Context, offsets map[string]int64) context.Context {
	return context.WithValue(ctx, resumeOffsetsKey{}, offsets)
}

// getResumeOffsets extracts resume offsets from context.
func getResumeOffsets(ctx context.Context) map[string]int64 {
	if offsets, ok := ctx.Value(resumeOffsetsKey{}).(map[string]int64); ok {
		return offsets
	}
	return nil
}

// rangeSuccessKey is a context key for storing successful Range requests.
type rangeSuccessKey struct{}

// RangeSuccess tracks which digests had successful Range requests.
type RangeSuccess struct {
	mu      sync.Mutex
	offsets map[string]int64 // digest -> successful offset
}

// Add records a successful Range request for a digest.
func (rs *RangeSuccess) Add(digest string, offset int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.offsets == nil {
		rs.offsets = make(map[string]int64)
	}
	rs.offsets[digest] = offset
}

// Get returns the successful offset for a digest, or 0 if not found.
func (rs *RangeSuccess) Get(digest string) (int64, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.offsets == nil {
		return 0, false
	}
	offset, ok := rs.offsets[digest]
	return offset, ok
}

// WithRangeSuccess adds a RangeSuccess tracker to a context.
func WithRangeSuccess(ctx context.Context, rs *RangeSuccess) context.Context {
	return context.WithValue(ctx, rangeSuccessKey{}, rs)
}

// GetRangeSuccess extracts RangeSuccess from context.
func GetRangeSuccess(ctx context.Context) *RangeSuccess {
	if rs, ok := ctx.Value(rangeSuccessKey{}).(*RangeSuccess); ok {
		return rs
	}
	return nil
}

// rangeTransport wraps an http.RoundTripper to add Range headers for resumable downloads
// and User-Agent headers for registry compatibility.
type rangeTransport struct {
	base      http.RoundTripper
	userAgent string
}

// RoundTrip implements http.RoundTripper, adding Range headers when resume offsets are present
// and User-Agent header when configured.
func (t *rangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	offsets := getResumeOffsets(req.Context())
	var requestedOffset int64
	var digest string

	if offsets != nil {
		digest, requestedOffset = t.extractDigestAndOffset(req, offsets)
	}

	// Clone request only once if we need to modify headers
	if t.userAgent != "" || requestedOffset > 0 {
		req = req.Clone(req.Context())
		if t.userAgent != "" {
			req.Header.Set("User-Agent", t.userAgent)
		}
		if requestedOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", requestedOffset))
		}
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// If we requested a Range, record success only if the server accepted the range request
	// Servers should return 206 (Partial Content) for successful range requests,
	// but some may return 200 with the partial content, so we record success for both
	if requestedOffset > 0 {
		if resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusOK {
			// Record in RangeSuccess tracker so WriteBlob can check it
			if rs := GetRangeSuccess(req.Context()); rs != nil {
				rs.Add(digest, requestedOffset)
			}
		}
		// If range request was not successful (e.g., 416 Range Not Satisfiable),
		// don't record in RangeSuccess, which will cause WriteBlob to start fresh
		// (no explicit action needed in the else case)
	}

	return resp, nil
}

// extractDigestAndOffset extracts the blob digest from the request URL and returns
// the corresponding resume offset if one exists.
func (t *rangeTransport) extractDigestAndOffset(req *http.Request, offsets map[string]int64) (string, int64) {
	// Parse digest from blob URL: /v2/<repo>/blobs/<digest>
	// The digest should be a valid OCI digest (e.g., sha256:abc123...)
	path := req.URL.Path
	if idx := strings.LastIndex(path, "/blobs/"); idx != -1 {
		digest := path[idx+7:] // len("/blobs/") = 7
		// Check if the extracted part looks like a valid digest
		if strings.Contains(digest, ":") { // Should contain algorithm:hash
			if offset, ok := offsets[digest]; ok {
				return digest, offset
			}
		}
	}

	// Also try to extract from query parameters (some registries might use this)
	if digest := req.URL.Query().Get("digest"); digest != "" {
		if offset, ok := offsets[digest]; ok {
			return digest, offset
		}
	}

	// Some registries might use different URL patterns, try to extract digest from path segments
	// Look for patterns like sha256:<hex> in the path
	pathSegments := strings.Split(path, "/")
	for _, segment := range pathSegments {
		if strings.Contains(segment, ":") { // Likely a digest format like sha256:abc123...
			if offset, ok := offsets[segment]; ok {
				return segment, offset
			}
		}
	}

	return "", 0
}

// makeOptions creates options from functional options.
func makeOptions(opts ...Option) *options {
	o := &options{
		ctx:       context.Background(),
		transport: DefaultTransport,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// credentialsFunc returns a docker credentials function.
func credentialsFunc(o *options, ref reference.Reference) func(string) (string, string, error) {
	return func(host string) (string, string, error) {
		var auth authn.Authenticator

		if o.auth != nil {
			auth = o.auth
		} else if o.keychain != nil {
			var err error
			auth, err = o.keychain.Resolve(authn.NewResource(ref))
			if err != nil {
				return "", "", err
			}
		}

		if auth == nil {
			return "", "", nil
		}

		cfg, err := auth.Authorization()
		if err != nil {
			return "", "", err
		}

		if cfg.RegistryToken != "" {
			return "", cfg.RegistryToken, nil
		}

		return cfg.Username, cfg.Password, nil
	}
}

// remoteImage implements oci.Image for remote images.
type remoteImage struct {
	ref         reference.Reference
	resolver    remotes.Resolver
	desc        v1.Descriptor
	manifest    *oci.Manifest
	rawManifest []byte
	store       content.Store
	ctx         context.Context
	mu          sync.Mutex
}

// resolverComponents holds the components created for a resolver.
type resolverComponents struct {
	resolver   remotes.Resolver
	authorizer docker.Authorizer
	httpClient *http.Client
	plainHTTP  bool
}

// createResolver creates a docker resolver with the given options.
func createResolver(o *options, ref reference.Reference) resolverComponents {
	authorizer := docker.NewDockerAuthorizer(
		docker.WithAuthCreds(credentialsFunc(o, ref)))

	// Wrap transport with Range header support for resumable downloads
	// and User-Agent header for registry compatibility (required by HuggingFace)
	transport := &rangeTransport{base: o.transport, userAgent: o.userAgent}
	client := &http.Client{Transport: transport}

	// Check if we should use plain HTTP (either explicitly configured or for insecure hosts)
	usePlainHTTP := o.plainHTTP || ref.Context().Registry.Scheme() == "http"

	var resolver remotes.Resolver
	if usePlainHTTP {
		// For plain HTTP, use a custom hosts function
		resolver = docker.NewResolver(docker.ResolverOptions{
			Hosts: func(host string) ([]docker.RegistryHost, error) {
				return []docker.RegistryHost{
					{
						Host:         host,
						Scheme:       "http",
						Path:         "/v2",
						Capabilities: docker.HostCapabilityPush | docker.HostCapabilityPull | docker.HostCapabilityResolve,
						Authorizer:   authorizer,
						Client:       client,
					},
				}, nil
			},
		})
	} else {
		resolver = docker.NewResolver(docker.ResolverOptions{
			Hosts: docker.ConfigureDefaultRegistries(
				docker.WithAuthorizer(authorizer),
				docker.WithClient(client)),
		})
	}

	return resolverComponents{
		resolver:   resolver,
		authorizer: authorizer,
		httpClient: client,
		plainHTTP:  usePlainHTTP,
	}
}

// createResolverWithPushScope creates a docker resolver pre-authorized with push scope.
func createResolverWithPushScope(o *options, ref reference.Reference) (resolverComponents, error) {
	var auth authn.Authenticator
	if o.auth != nil {
		auth = o.auth
	} else if o.keychain != nil {
		var err error
		auth, err = o.keychain.Resolve(authn.NewResource(ref))
		if err != nil {
			return resolverComponents{}, fmt.Errorf("resolving credentials: %w", err)
		}
	}

	usePlainHTTP := o.plainHTTP || ref.Context().Registry.Scheme() == "http"

	// If no auth is needed or using plain HTTP, use the standard resolver
	if auth == nil || usePlainHTTP {
		return createResolver(o, ref), nil
	}

	// Pre-authorize with push scope
	pr, err := Ping(o.ctx, ref.Context().Registry, o.transport)
	if err != nil {
		// Ping failed, fall back to standard resolver
		return createResolver(o, ref), nil
	}

	// If no WWW-Authenticate header, no token exchange needed
	if pr.WWWAuthenticate.Realm == "" {
		return createResolver(o, ref), nil
	}

	// Exchange credentials for a token with push scope
	scope := ref.Scope(PushScope)
	tok, err := Exchange(o.ctx, ref.Context().Registry, auth, o.transport,
		[]string{scope}, pr)
	if err != nil {
		// Token exchange failed, fall back to standard resolver
		return createResolver(o, ref), nil
	}

	// Create transport with the bearer token
	bearerTransport := &BearerTransport{
		Transport: &rangeTransport{base: o.transport, userAgent: o.userAgent},
		Token:     tok.Token,
	}
	client := &http.Client{Transport: bearerTransport}

	// Create resolver with the pre-authorized token
	// We keep the original auth available for re-challenges (e.g., token expiry, additional scope)
	// The BearerTransport will handle the primary auth, but if challenged, we can re-exchange
	authorizer := docker.NewDockerAuthorizer(
		docker.WithAuthCreds(func(host string) (string, string, error) {
			// Return original credentials to handle potential re-challenges
			// (token refresh, additional scope requests)
			cfg, err := auth.Authorization()
			if err != nil {
				return "", "", err
			}
			if cfg.RegistryToken != "" {
				return "", cfg.RegistryToken, nil
			}
			return cfg.Username, cfg.Password, nil
		}))

	resolver := docker.NewResolver(docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(
			docker.WithAuthorizer(authorizer),
			docker.WithClient(client)),
	})

	return resolverComponents{
		resolver:   resolver,
		authorizer: authorizer,
		httpClient: client,
		plainHTTP:  usePlainHTTP,
	}, nil
}

// Image fetches a remote image.
func Image(ref reference.Reference, opts ...Option) (oci.Image, error) {
	o := makeOptions(opts...)

	// Create resolver
	components := createResolver(o, ref)

	// Resolve the reference
	name, desc, err := components.resolver.Resolve(o.ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref.String(), err)
	}
	_ = name // we use the original ref

	// Create a temporary content store
	tmpDir, err := os.MkdirTemp("", "model-runner-remote")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}

	store, err := local.NewStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("creating content store: %w", err)
	}

	return &remoteImage{
		ref:      ref,
		resolver: components.resolver,
		desc:     desc,
		store:    store,
		ctx:      o.ctx,
	}, nil
}

// fetchManifest fetches and caches the manifest.
func (i *remoteImage) fetchManifest() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.manifest != nil {
		return nil
	}

	fetcher, err := i.resolver.Fetcher(i.ctx, i.ref.String())
	if err != nil {
		return fmt.Errorf("getting fetcher: %w", err)
	}

	// Fetch manifest
	rc, err := fetcher.Fetch(i.ctx, i.desc)
	if err != nil {
		return fmt.Errorf("fetching manifest: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	i.rawManifest = data

	var manifest oci.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	i.manifest = &manifest
	return nil
}

// Layers returns the image layers.
func (i *remoteImage) Layers() ([]oci.Layer, error) {
	if err := i.fetchManifest(); err != nil {
		return nil, err
	}

	layers := make([]oci.Layer, len(i.manifest.Layers))
	for idx, desc := range i.manifest.Layers {
		layers[idx] = &remoteLayer{
			image: i,
			desc:  desc,
			index: idx,
		}
	}
	return layers, nil
}

// MediaType returns the manifest media type.
func (i *remoteImage) MediaType() (oci.MediaType, error) {
	if err := i.fetchManifest(); err != nil {
		return "", err
	}
	return i.manifest.MediaType, nil
}

// Size returns the manifest size.
func (i *remoteImage) Size() (int64, error) {
	return i.desc.Size, nil
}

// ConfigName returns the config digest.
func (i *remoteImage) ConfigName() (oci.Hash, error) {
	if err := i.fetchManifest(); err != nil {
		return oci.Hash{}, err
	}
	return i.manifest.Config.Digest, nil
}

// ConfigFile returns the parsed config file.
func (i *remoteImage) ConfigFile() (*oci.ConfigFile, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, err
	}

	var cfg oci.ConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// RawConfigFile returns the raw config bytes.
func (i *remoteImage) RawConfigFile() ([]byte, error) {
	if err := i.fetchManifest(); err != nil {
		return nil, err
	}

	fetcher, err := i.resolver.Fetcher(i.ctx, i.ref.String())
	if err != nil {
		return nil, fmt.Errorf("getting fetcher: %w", err)
	}

	configDesc := v1.Descriptor{
		MediaType: string(i.manifest.Config.MediaType),
		Digest:    godigest.Digest(i.manifest.Config.Digest.String()),
		Size:      i.manifest.Config.Size,
	}

	rc, err := fetcher.Fetch(i.ctx, configDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching config: %w", err)
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

// Digest returns the manifest digest.
func (i *remoteImage) Digest() (oci.Hash, error) {
	return oci.FromDigest(i.desc.Digest), nil
}

// Manifest returns the manifest.
func (i *remoteImage) Manifest() (*oci.Manifest, error) {
	if err := i.fetchManifest(); err != nil {
		return nil, err
	}
	return i.manifest, nil
}

// RawManifest returns the raw manifest bytes.
func (i *remoteImage) RawManifest() ([]byte, error) {
	if err := i.fetchManifest(); err != nil {
		return nil, err
	}
	return i.rawManifest, nil
}

// LayerByDigest returns a layer by its digest.
func (i *remoteImage) LayerByDigest(h oci.Hash) (oci.Layer, error) {
	layers, err := i.Layers()
	if err != nil {
		return nil, err
	}

	for _, l := range layers {
		d, err := l.Digest()
		if err != nil {
			continue
		}
		if d.String() == h.String() {
			return l, nil
		}
	}

	return nil, fmt.Errorf("layer not found: %s", h.String())
}

// LayerByDiffID returns a layer by its diff ID.
func (i *remoteImage) LayerByDiffID(h oci.Hash) (oci.Layer, error) {
	// For remote images, we typically use digest
	return i.LayerByDigest(h)
}

// remoteLayer implements oci.Layer for remote layers.
type remoteLayer struct {
	image *remoteImage
	desc  oci.Descriptor
	index int // Index of this layer in the manifest
}

// Digest returns the layer digest.
func (l *remoteLayer) Digest() (oci.Hash, error) {
	return l.desc.Digest, nil
}

// DiffID returns the uncompressed layer digest.
// For remote layers, we look up the diff ID from the image config.
func (l *remoteLayer) DiffID() (oci.Hash, error) {
	// Get the config file to look up the diff ID
	config, err := l.image.ConfigFile()
	if err != nil {
		return oci.Hash{}, fmt.Errorf("getting config file for diff ID lookup: %w", err)
	}

	// Check if the layer index is within bounds of the diff IDs
	if l.index < 0 || l.index >= len(config.RootFS.DiffIDs) {
		return l.desc.Digest, nil // Fallback to digest if diff ID not available
	}

	return config.RootFS.DiffIDs[l.index], nil
}

// Compressed returns the compressed layer contents.
func (l *remoteLayer) Compressed() (io.ReadCloser, error) {
	fetcher, err := l.image.resolver.Fetcher(l.image.ctx, l.image.ref.String())
	if err != nil {
		return nil, fmt.Errorf("getting fetcher: %w", err)
	}

	desc := v1.Descriptor{
		MediaType: string(l.desc.MediaType),
		Digest:    godigest.Digest(l.desc.Digest.String()),
		Size:      l.desc.Size,
	}

	return fetcher.Fetch(l.image.ctx, desc)
}

// Uncompressed returns the uncompressed layer contents.
func (l *remoteLayer) Uncompressed() (io.ReadCloser, error) {
	// For simplicity, return compressed data
	// Real implementations would decompress based on media type
	return l.Compressed()
}

// Size returns the compressed layer size.
func (l *remoteLayer) Size() (int64, error) {
	return l.desc.Size, nil
}

// MediaType returns the layer media type.
func (l *remoteLayer) MediaType() (oci.MediaType, error) {
	return l.desc.MediaType, nil
}

// Write pushes an image to a registry.
func Write(ref reference.Reference, img oci.Image, opts ...Option) error {
	o := makeOptions(opts...)

	// Pre-authorize with push scope to ensure we have the right permissions
	components, err := createResolverWithPushScope(o, ref)
	if err != nil {
		return fmt.Errorf("creating resolver with push scope: %w", err)
	}

	// Get pusher
	pusher, err := components.resolver.Pusher(o.ctx, ref.String())
	if err != nil {
		return fmt.Errorf("getting pusher: %w", err)
	}

	// Push layers first
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	var totalSize int64
	for _, layer := range layers {
		size, err := layer.Size()
		if err != nil {
			return fmt.Errorf("getting layer size: %w", err)
		}
		totalSize += size
	}

	var completed int64
	for _, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer digest: %w", err)
		}

		size, err := layer.Size()
		if err != nil {
			return fmt.Errorf("getting layer size: %w", err)
		}

		mt, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("getting layer media type: %w", err)
		}

		desc := v1.Descriptor{
			MediaType: string(mt),
			Digest:    godigest.Digest(digest.String()),
			Size:      size,
		}

		rc, err := layer.Compressed()
		if err != nil {
			return fmt.Errorf("getting layer content: %w", err)
		}

		// Create content writer for push
		cw, err := pusher.Push(o.ctx, desc)
		if err != nil {
			rc.Close()
			// If already exists, continue
			if errdefs.IsAlreadyExists(err) || strings.Contains(err.Error(), "already exists") {
				completed += size
				if o.progress != nil {
					o.progress <- oci.Update{
						Complete: completed,
						Total:    totalSize,
					}
				}
				continue
			}
			closeProgress(o.progress)
			return fmt.Errorf("pushing layer: %w", err)
		}

		if _, err := io.Copy(cw, rc); err != nil {
			cw.Close()
			rc.Close()
			closeProgress(o.progress)
			return fmt.Errorf("writing layer: %w", err)
		}

		if err := cw.Commit(o.ctx, size, desc.Digest); err != nil {
			cw.Close()
			rc.Close()
			if !errdefs.IsAlreadyExists(err) && !strings.Contains(err.Error(), "already exists") {
				closeProgress(o.progress)
				return fmt.Errorf("committing layer: %w", err)
			}
			// If it already exists, we still want to update progress
			completed += size
			if o.progress != nil {
				o.progress <- oci.Update{
					Complete: completed,
					Total:    totalSize,
				}
			}
		} else {
			// Successfully committed, update progress
			completed += size
			if o.progress != nil {
				o.progress <- oci.Update{
					Complete: completed,
					Total:    totalSize,
				}
			}
		}
		cw.Close()
		rc.Close()
	}

	// Push config
	rawConfig, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("getting config: %w", err)
	}

	configName, err := img.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}

	configDesc := v1.Descriptor{
		MediaType: "application/vnd.docker.container.image.v1+json",
		Digest:    godigest.Digest(configName.String()),
		Size:      int64(len(rawConfig)),
	}

	cw, err := pusher.Push(o.ctx, configDesc)
	if err != nil {
		if !errdefs.IsAlreadyExists(err) && !strings.Contains(err.Error(), "already exists") {
			closeProgress(o.progress)
			return fmt.Errorf("pushing config: %w", err)
		}
		// If it already exists, we don't have a writer to close, just continue
	} else {
		if _, err := cw.Write(rawConfig); err != nil {
			cw.Close()
			closeProgress(o.progress)
			return fmt.Errorf("writing config: %w", err)
		}
		if err := cw.Commit(o.ctx, int64(len(rawConfig)), configDesc.Digest); err != nil {
			cw.Close()
			if !errdefs.IsAlreadyExists(err) && !strings.Contains(err.Error(), "already exists") {
				closeProgress(o.progress)
				return fmt.Errorf("committing config: %w", err)
			}
		}
		cw.Close()
	}

	// Push manifest
	rawManifest, err := img.RawManifest()
	if err != nil {
		closeProgress(o.progress)
		return fmt.Errorf("getting manifest: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		closeProgress(o.progress)
		return fmt.Errorf("getting manifest object: %w", err)
	}

	manifestDigest, err := img.Digest()
	if err != nil {
		closeProgress(o.progress)
		return fmt.Errorf("getting manifest digest: %w", err)
	}

	manifestDesc := v1.Descriptor{
		MediaType: string(manifest.MediaType),
		Digest:    godigest.Digest(manifestDigest.String()),
		Size:      int64(len(rawManifest)),
	}

	cw, err = pusher.Push(o.ctx, manifestDesc)
	if err != nil {
		if !errdefs.IsAlreadyExists(err) && !strings.Contains(err.Error(), "already exists") {
			closeProgress(o.progress)
			return fmt.Errorf("pushing manifest: %w", err)
		}
		// If it already exists, we don't have a writer to close, just continue
		// If it already exists, we still want to close progress and return success
		closeProgress(o.progress)
		return nil
	}

	if _, err := cw.Write(rawManifest); err != nil {
		cw.Close()
		closeProgress(o.progress)
		return fmt.Errorf("writing manifest: %w", err)
	}

	if err := cw.Commit(o.ctx, int64(len(rawManifest)), manifestDesc.Digest); err != nil {
		cw.Close()
		closeProgress(o.progress)
		if !errdefs.IsAlreadyExists(err) && !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("committing manifest: %w", err)
		}
		// If it already exists, we still want to close the writer
		cw.Close()
	}
	cw.Close()

	// Close progress channel to signal completion
	closeProgress(o.progress)

	return nil
}

// closeProgress safely closes the progress channel if not nil
func closeProgress(ch chan<- oci.Update) {
	if ch != nil {
		close(ch)
	}
}

// Ensure remoteImage is cleaned up properly
func (i *remoteImage) Close() error {
	// The local content store doesn't expose its root path, so cleanup
	// of temp directories should be handled by the caller.
	return nil
}

// Helper to configure the resolver for operations
func configureResolver(o *options, ref reference.Reference) remotes.Resolver {
	// Use the same logic as createResolver for consistency
	return createResolver(o, ref).resolver
}

// Descriptor returns a descriptor for a remote reference without fetching the full manifest.
func Descriptor(ref reference.Reference, opts ...Option) (*oci.Descriptor, error) {
	o := makeOptions(opts...)
	resolver := configureResolver(o, ref)

	_, desc, err := resolver.Resolve(o.ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref.String(), err)
	}

	return &oci.Descriptor{
		MediaType: oci.MediaType(desc.MediaType),
		Size:      desc.Size,
		Digest:    oci.FromDigest(desc.Digest),
	}, nil
}

// FetchHandler wraps containerd's FetchHandler for custom progress tracking.
func FetchHandler(store content.Store, fetcher remotes.Fetcher) images.Handler {
	return remotes.FetchHandler(store, fetcher)
}
