package dockerhub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	remoteerrors "github.com/containerd/containerd/v2/core/remotes/errors"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/docker/model-runner/pkg/internal/registryutil"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// PullPlatform downloads image for the given OS/architecture and writes it to
// destination as a tarball. Mirrors are tried before registry-1.docker.io for
// Docker Hub references. When creds is nil, credentials are resolved from the
// environment and ~/.docker/config.json.
func PullPlatform(ctx context.Context, image, destination, requiredOs, requiredArch string, mirrors []string, creds Credentials) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("creating destination directory %s: %w", filepath.Dir(destination), err)
	}
	output, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("creating destination file %s: %w", destination, err)
	}
	tmpDir, err := os.MkdirTemp("", "docker-pull")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	store, err := local.NewStore(tmpDir)
	if err != nil {
		return fmt.Errorf("creating new content store: %w", err)
	}
	resolver := newResolver(mirrors, creds)
	desc, err := retry(ctx, 10, 1*time.Second, func() (*v1.Descriptor, error) {
		return fetch(ctx, resolver, store, image, requiredOs, requiredArch)
	})
	if err != nil {
		return fmt.Errorf("fetching image: %w", err)
	}
	return archive.Export(ctx, store, output, archive.WithManifest(*desc, image), archive.WithSkipMissing(store))
}

// ResolveDigest resolves the given image reference (e.g. "registry-1.docker.io/docker/foo:tag")
// against the registry (with optional mirrors tried first for Docker Hub references) and
// returns the resolved digest. It does not download any blobs; it issues only the manifest
// HEAD/GET that the registry resolver needs.
//
// Authentication uses the same credentials lookup as PullPlatform: creds when
// non-nil, otherwise the environment and ~/.docker/config.json (including
// credHelpers and credsStore), so a prior `docker login <mirror-host>` is
// honored.
func ResolveDigest(ctx context.Context, ref string, mirrors []string, creds Credentials) (string, error) {
	resolver := newResolver(mirrors, creds)
	desc, err := retry(ctx, 10, 1*time.Second, func() (*v1.Descriptor, error) {
		name, d, err := resolver.Resolve(ctx, ref)
		if err != nil {
			return nil, err
		}
		slog.Debug("resolved image tag", "ref", ref, "resolved", name, "digest", d.Digest.String())
		return &d, nil
	})
	if err != nil {
		return "", fmt.Errorf("resolving image %q: %w", ref, err)
	}
	return desc.Digest.String(), nil
}

// newResolver builds a containerd docker resolver that tries the given mirrors
// before the upstream registry, authenticating with creds — or, when creds is
// nil, with defaultCredentials.
func newResolver(mirrors []string, creds Credentials) remotes.Resolver {
	if creds == nil {
		creds = defaultCredentials
	}
	authorizer := docker.NewDockerAuthorizer(docker.WithAuthCreds(creds))
	return docker.NewResolver(docker.ResolverOptions{
		Hosts: registryutil.RegistryHosts(mirrors, authorizer, nil),
	})
}

func retry(ctx context.Context, attempts int, sleep time.Duration, f func() (*v1.Descriptor, error)) (*v1.Descriptor, error) {
	var err error
	var result *v1.Descriptor
	for i := 0; i < attempts; i++ {
		if i > 0 {
			slog.Info("retrying after error", "attempt", i, "error", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleep):
			}
		}
		result, err = f()
		if err == nil {
			return result, nil
		}
		if isTerminal(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts, last error: %w", attempts, err)
}

// isTerminal reports whether err is non-retryable: a missing tag/manifest, an
// authentication/authorization failure, or a canceled/expired context. Retrying
// these only wastes time, so the caller should fail fast instead of looping.
//
// The containerd resolver only maps 404 to errdefs.ErrNotFound; other 4xx
// statuses (including 401 and 403) surface as a remoteerrors.ErrUnexpectedStatus
// carrying the raw status code, so we inspect that explicitly. 429 is
// deliberately left retryable — the resolver already retries it internally and a
// later attempt can succeed once a rate limit clears.
func isTerminal(err error) bool {
	if errdefs.IsNotFound(err) ||
		errdefs.IsUnauthorized(err) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var unexpected remoteerrors.ErrUnexpectedStatus
	if errors.As(err, &unexpected) {
		switch unexpected.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return true
		}
	}
	return false
}

func fetch(ctx context.Context, resolver remotes.Resolver, store content.Store, ref, requiredOs, requiredArch string) (*v1.Descriptor, error) {
	name, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	fetcher, err := resolver.Fetcher(ctx, name)
	if err != nil {
		return nil, err
	}

	childrenHandler := images.ChildrenHandler(store)
	if requiredOs != "" && requiredArch != "" {
		requiredPlatform := platforms.Only(v1.Platform{OS: requiredOs, Architecture: requiredArch})
		childrenHandler = images.LimitManifests(images.FilterPlatforms(images.ChildrenHandler(store), requiredPlatform), requiredPlatform, 1)
	}
	h := images.Handlers(remotes.FetchHandler(store, fetcher), childrenHandler)
	if err := images.Dispatch(ctx, h, nil, desc); err != nil {
		return nil, err
	}
	return &desc, nil
}
