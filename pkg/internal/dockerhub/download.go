package dockerhub

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/docker/model-runner/pkg/internal/jsonutil"
	"github.com/docker/model-runner/pkg/internal/registryutil"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func PullPlatform(ctx context.Context, image, destination, requiredOs, requiredArch string, mirrors []string) error {
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
	resolver := newResolver(mirrors)
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
// Authentication uses the same credentials lookup as PullPlatform (env vars
// DOCKER_HUB_USER/DOCKER_HUB_PASSWORD or ~/.docker/config.json), so a prior
// `docker login <mirror-host>` is honored.
func ResolveDigest(ctx context.Context, ref string, mirrors []string) (string, error) {
	resolver := newResolver(mirrors)
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

// newResolver builds a containerd docker resolver that authenticates via
// dockerCredentials and tries the given mirrors before the upstream registry.
func newResolver(mirrors []string) remotes.Resolver {
	authorizer := docker.NewDockerAuthorizer(docker.WithAuthCreds(dockerCredentials))
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

func dockerCredentials(host string) (string, string, error) {
	hubUsername, hubPassword := os.Getenv("DOCKER_HUB_USER"), os.Getenv("DOCKER_HUB_PASSWORD")
	if hubUsername != "" && hubPassword != "" {
		return hubUsername, hubPassword, nil
	}
	slog.Debug("checking for registry auth config", "host", host)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	credentialConfig := filepath.Join(home, ".docker", "config.json")
	cfg := struct {
		Auths map[string]struct {
			Auth string
		}
	}{}
	if err := jsonutil.ReadFile(credentialConfig, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", nil
		}
		return "", "", err
	}
	for h, r := range cfg.Auths {
		if h == host {
			creds, err := base64.StdEncoding.DecodeString(r.Auth)
			if err != nil {
				return "", "", err
			}
			parts := strings.SplitN(string(creds), ":", 2)
			if len(parts) != 2 {
				slog.Debug("skipping non-user/password auth for registry", "host", host, "auth_type", parts[0])
				return "", "", nil
			}
			slog.Debug("using auth for registry", "host", host, "user", parts[0])
			return parts[0], parts[1], nil
		}
	}
	return "", "", nil
}
