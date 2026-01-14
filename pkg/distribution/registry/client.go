package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/authn"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
	"github.com/docker/model-runner/pkg/distribution/types"
)

const (
	DefaultUserAgent = "model-distribution"
)

var (
	defaultRegistryOpts []reference.Option
	once                sync.Once
	DefaultTransport    = remote.DefaultTransport
)

// GetDefaultRegistryOptions returns reference.Option slice with custom default registry
// and insecure flag if the corresponding environment variables are set.
// Environment variables are read once at first call and cached for consistency.
// Returns a copy of the options to prevent race conditions from slice modifications.
// - DEFAULT_REGISTRY: Override the default registry (index.docker.io)
// - INSECURE_REGISTRY: Set to "true" to allow HTTP connections
func GetDefaultRegistryOptions() []reference.Option {
	once.Do(func() {
		var opts []reference.Option
		if defaultReg := os.Getenv("DEFAULT_REGISTRY"); defaultReg != "" {
			opts = append(opts, reference.WithDefaultRegistry(defaultReg))
		}
		if os.Getenv("INSECURE_REGISTRY") == "true" {
			opts = append(opts, reference.Insecure)
		}
		// Always use the default org for consistency with model-runner's normalization
		opts = append(opts, reference.WithDefaultOrg(reference.DefaultOrg))
		defaultRegistryOpts = opts
	})
	return append([]reference.Option(nil), defaultRegistryOpts...)
}

type Client struct {
	transport http.RoundTripper
	userAgent string
	keychain  authn.Keychain
	auth      authn.Authenticator
	plainHTTP bool
}

type ClientOption func(*Client)

func WithTransport(transport http.RoundTripper) ClientOption {
	return func(c *Client) {
		if transport != nil {
			c.transport = transport
		}
	}
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

func WithAuthConfig(username, password string) ClientOption {
	return func(c *Client) {
		if username != "" && password != "" {
			c.auth = &authn.Basic{
				Username: username,
				Password: password,
			}
		}
	}
}

// WithAuth sets a custom authenticator.
func WithAuth(auth authn.Authenticator) ClientOption {
	return func(c *Client) {
		if auth != nil {
			c.auth = auth
		}
	}
}

// WithPlainHTTP enables or disables plain HTTP connections to registries.
func WithPlainHTTP(plain bool) ClientOption {
	return func(c *Client) {
		c.plainHTTP = plain
	}
}

func NewClient(opts ...ClientOption) *Client {
	client := &Client{
		transport: remote.DefaultTransport,
		userAgent: DefaultUserAgent,
		keychain:  authn.DefaultKeychain,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// FromClient creates a new Client by copying an existing client's configuration
// and applying optional modifications via ClientOption functions.
func FromClient(base *Client, opts ...ClientOption) *Client {
	client := &Client{
		transport: base.transport,
		userAgent: base.userAgent,
		keychain:  base.keychain,
		auth:      base.auth,
		plainHTTP: base.plainHTTP,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func (c *Client) Model(ctx context.Context, ref string) (types.ModelArtifact, error) {
	// Parse the reference
	parsedRef, err := reference.ParseReference(ref, GetDefaultRegistryOptions()...)
	if err != nil {
		return nil, NewReferenceError(ref, err)
	}

	// Set up authentication options
	authOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(c.transport),
		remote.WithUserAgent(c.userAgent),
		remote.WithPlainHTTP(c.plainHTTP),
	}

	// Use direct auth if provided, otherwise fall back to keychain
	if c.auth != nil {
		authOpts = append(authOpts, remote.WithAuth(c.auth))
	} else {
		authOpts = append(authOpts, remote.WithAuthFromKeychain(c.keychain))
	}

	// Return the artifact at the given reference
	remoteImg, err := remote.Image(parsedRef, authOpts...)
	if err != nil {
		errStr := err.Error()
		errStrLower := strings.ToLower(errStr)
		if strings.Contains(errStr, "UNAUTHORIZED") || strings.Contains(errStrLower, "unauthorized") {
			return nil, NewRegistryError(ref, "UNAUTHORIZED", "Authentication required for this model", err)
		}
		if strings.Contains(errStr, "MANIFEST_UNKNOWN") {
			return nil, NewRegistryError(ref, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		if strings.Contains(errStr, "NAME_UNKNOWN") {
			return nil, NewRegistryError(ref, "NAME_UNKNOWN", "Repository not found", err)
		}
		// containerd resolver returns "404 Not Found" or "not found" for missing manifests
		if strings.Contains(errStr, "404") || strings.Contains(errStrLower, "not found") {
			return nil, NewRegistryError(ref, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		// containerd resolver may return different error formats - check for common patterns
		if strings.Contains(errStrLower, "manifest unknown") ||
			strings.Contains(errStrLower, "name unknown") ||
			strings.Contains(errStrLower, "blob unknown") {
			return nil, NewRegistryError(ref, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		// Preserve the original error for API consumers to handle appropriately
		return nil, NewRegistryError(ref, "UNKNOWN", err.Error(), err)
	}

	return &artifact{remoteImg}, nil
}

func (c *Client) BlobURL(ref string, digest oci.Hash) (string, error) {
	// Parse the reference
	parsedRef, err := reference.ParseReference(ref, GetDefaultRegistryOptions()...)
	if err != nil {
		return "", NewReferenceError(ref, err)
	}

	return fmt.Sprintf("%s://%s/v2/%s/blobs/%s",
		parsedRef.Context().Registry.Scheme(),
		parsedRef.Context().Registry.RegistryStr(),
		parsedRef.Context().RepositoryStr(),
		digest.String()), nil
}

func (c *Client) BearerToken(ctx context.Context, ref string) (string, error) {
	// Parse the reference
	parsedRef, err := reference.ParseReference(ref, GetDefaultRegistryOptions()...)
	if err != nil {
		return "", NewReferenceError(ref, err)
	}

	var auth authn.Authenticator
	if c.auth != nil {
		auth = c.auth
	} else {
		auth, err = c.keychain.Resolve(authn.NewResource(parsedRef))
		if err != nil {
			return "", fmt.Errorf("resolving credentials: %w", err)
		}
	}

	pr, err := remote.Ping(ctx, parsedRef.Context().Registry, c.transport)
	if err != nil {
		return "", fmt.Errorf("pinging registry: %w", err)
	}

	tok, err := remote.Exchange(ctx, parsedRef.Context().Registry, auth, c.transport, []string{parsedRef.Scope(remote.PullScope)}, pr)
	if err != nil {
		return "", fmt.Errorf("getting registry token: %w", err)
	}
	return tok.Token, nil
}

type Target struct {
	reference reference.Reference
	transport http.RoundTripper
	userAgent string
	keychain  authn.Keychain
	auth      authn.Authenticator
	plainHTTP bool
}

func (c *Client) NewTarget(tag string) (*Target, error) {
	ref, err := reference.NewTag(tag, GetDefaultRegistryOptions()...)
	if err != nil {
		return nil, fmt.Errorf("invalid tag: %q: %w", tag, err)
	}
	return &Target{
		reference: ref,
		transport: c.transport,
		userAgent: c.userAgent,
		keychain:  c.keychain,
		auth:      c.auth,
		plainHTTP: c.plainHTTP,
	}, nil
}

func (t *Target) Write(ctx context.Context, model types.ModelArtifact, progressWriter io.Writer) error {
	layers, err := model.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	imageSize := int64(0)
	for _, layer := range layers {
		size, err := layer.Size()
		if err != nil {
			return fmt.Errorf("getting layer size: %w", err)
		}
		imageSize += size
	}

	// Set up authentication options
	authOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(t.transport),
		remote.WithUserAgent(t.userAgent),
		remote.WithPlainHTTP(t.plainHTTP),
	}

	// Use direct auth if provided, otherwise fall back to keychain
	if t.auth != nil {
		authOpts = append(authOpts, remote.WithAuth(t.auth))
	} else {
		authOpts = append(authOpts, remote.WithAuthFromKeychain(t.keychain))
	}

	if err := remote.Write(t.reference, model, progressWriter, authOpts...); err != nil {
		return fmt.Errorf("write to registry %q: %w", t.reference.String(), err)
	}
	return nil
}
