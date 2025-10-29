package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/types"
)

const (
	DefaultUserAgent = "model-distribution"
)

var (
	DefaultTransport = remote.DefaultTransport
)

type Client struct {
	transport http.RoundTripper
	userAgent string
	keychain  authn.Keychain
	auth      authn.Authenticator
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

func (c *Client) Model(ctx context.Context, reference string) (types.ModelArtifact, error) {
	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return nil, NewReferenceError(reference, err)
	}

	// Set up authentication options
	authOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(c.transport),
		remote.WithUserAgent(c.userAgent),
	}

	// Use direct auth if provided, otherwise fall back to keychain
	if c.auth != nil {
		authOpts = append(authOpts, remote.WithAuth(c.auth))
	} else {
		authOpts = append(authOpts, remote.WithAuthFromKeychain(c.keychain))
	}

	// Return the artifact at the given reference
	remoteImg, err := remote.Image(ref, authOpts...)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "UNAUTHORIZED") {
			return nil, NewRegistryError(reference, "UNAUTHORIZED", "Authentication required for this model", err)
		}
		if strings.Contains(errStr, "MANIFEST_UNKNOWN") {
			return nil, NewRegistryError(reference, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		if strings.Contains(errStr, "NAME_UNKNOWN") {
			return nil, NewRegistryError(reference, "NAME_UNKNOWN", "Repository not found", err)
		}
		return nil, NewRegistryError(reference, "UNKNOWN", err.Error(), err)
	}

	return &artifact{remoteImg}, nil
}

func (c *Client) BlobURL(reference string, digest v1.Hash) (string, error) {
	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", NewReferenceError(reference, err)
	}

	return fmt.Sprintf("%s://%s/v2/%s/blobs/%s",
		ref.Context().Registry.Scheme(),
		ref.Context().Registry.RegistryStr(),
		ref.Context().RepositoryStr(),
		digest.String()), nil
}

func (c *Client) BearerToken(ctx context.Context, reference string) (string, error) {
	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", NewReferenceError(reference, err)
	}

	var auth authn.Authenticator
	if c.auth != nil {
		auth = c.auth
	} else {
		auth, err = c.keychain.Resolve(ref.Context())
		if err != nil {
			return "", fmt.Errorf("resolving credentials: %w", err)
		}
	}

	pr, err := transport.Ping(ctx, ref.Context().Registry, c.transport)
	if err != nil {
		return "", fmt.Errorf("pinging registry: %w", err)
	}

	tok, err := transport.Exchange(ctx, ref.Context().Registry, auth, c.transport, []string{ref.Scope(transport.PullScope)}, pr)
	if err != nil {
		return "", fmt.Errorf("getting registry token: %w", err)
	}
	return tok.Token, nil
}

type Target struct {
	reference name.Reference
	transport http.RoundTripper
	userAgent string
	keychain  authn.Keychain
	auth      authn.Authenticator
}

func (c *Client) NewTarget(tag string) (*Target, error) {
	ref, err := name.NewTag(tag)
	if err != nil {
		return nil, fmt.Errorf("invalid tag: %q: %w", tag, err)
	}
	return &Target{
		reference: ref,
		transport: c.transport,
		userAgent: c.userAgent,
		keychain:  c.keychain,
		auth:      c.auth,
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
	pr := progress.NewProgressReporter(progressWriter, progress.PushMsg, imageSize, nil)
	defer pr.Wait()

	// Set up authentication options
	authOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(t.transport),
		remote.WithUserAgent(t.userAgent),
		remote.WithProgress(pr.Updates()),
	}

	// Use direct auth if provided, otherwise fall back to keychain
	if t.auth != nil {
		authOpts = append(authOpts, remote.WithAuth(t.auth))
	} else {
		authOpts = append(authOpts, remote.WithAuthFromKeychain(t.keychain))
	}

	if err := remote.Write(t.reference, model, authOpts...); err != nil {
		return fmt.Errorf("write to registry %q: %w", t.reference.String(), err)
	}
	return nil
}
