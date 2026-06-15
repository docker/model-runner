package dockerhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// registryHandler is a minimal Docker Registry v2 HTTP handler that supports
// the manifest HEAD / GET requests issued by containerd's docker resolver.
type registryHandler struct {
	// tag is the tag to recognize; for any other tag the handler returns 404.
	tag string
	// digest returned in the Docker-Content-Digest header.
	digest string
	// requests counts how many requests this handler received (for assertions).
	requests atomic.Int64
}

func (h *registryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests.Add(1)
	switch {
	case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
		// API version probe.
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
	case strings.HasSuffix(r.URL.Path, "/manifests/"+h.tag):
		// Manifest HEAD/GET for the recognized tag.
		w.Header().Set("Docker-Content-Digest", h.digest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		body := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// TestResolveDigest_UsesMirror verifies that when a mirror is configured for
// Docker Hub references, the resolver issues its manifest lookup against the
// mirror rather than registry-1.docker.io. This is the path enterprise
// customers behind an Artifactory / Nexus / Harbor mirror need.
func TestResolveDigest_UsesMirror(t *testing.T) {
	const wantDigest = "sha256:48883a67000000000000000000000000000000000000000000000000deadbeef"

	mirror := &registryHandler{tag: "latest-cuda", digest: wantDigest}
	srv := httptest.NewServer(mirror)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Reference points at registry-1.docker.io; the mirror should intercept it.
	ref := "registry-1.docker.io/docker/docker-model-backend-llamacpp:latest-cuda"
	got, err := ResolveDigest(ctx, ref, []string{srv.URL})
	if err != nil {
		t.Fatalf("ResolveDigest returned error: %v", err)
	}
	if got != wantDigest {
		t.Fatalf("digest mismatch: got %q want %q", got, wantDigest)
	}
	if mirror.requests.Load() == 0 {
		t.Fatalf("expected mirror to be called at least once, got 0 requests")
	}
}

// TestResolveDigest_CanceledContext verifies the resolver does not block when
// the context is already canceled. This protects against silent stalls when
// the network path to the upstream/mirror is blackholed (a frequent symptom
// in enterprise networks).
func TestResolveDigest_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// No mirror, no real network call should complete. We bound the test
	// with a wall-clock deadline so a regression cannot hang CI. A canceled
	// context is classified as terminal, so retry must not loop.
	done := make(chan struct{})
	var resolveErr error
	go func() {
		_, resolveErr = ResolveDigest(ctx, "registry-1.docker.io/docker/docker-model-backend-llamacpp:latest-cuda", nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("ResolveDigest did not return on canceled context within 5s")
	}
	if resolveErr == nil {
		t.Fatalf("expected error on canceled context, got nil")
	}
}

// TestRetry_FailsFastOnTerminalError verifies retry does not loop on a
// non-retryable error (e.g. a missing tag / 404). Before this, every error was
// retried 10 times with 1s sleeps (~9s), blocking the install/startup path.
func TestRetry_FailsFastOnTerminalError(t *testing.T) {
	var calls int
	_, err := retry(t.Context(), 10, time.Second, func() (*v1.Descriptor, error) {
		calls++
		return nil, errdefs.ErrNotFound
	})
	if err == nil {
		t.Fatalf("expected error on terminal failure, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt on a terminal error, got %d", calls)
	}
}

// TestRetry_RetriesTransientError verifies retry still loops the full budget on
// an unclassified (transient) error, preserving the original behavior.
func TestRetry_RetriesTransientError(t *testing.T) {
	var calls int
	_, err := retry(t.Context(), 3, time.Millisecond, func() (*v1.Descriptor, error) {
		calls++
		return nil, errors.New("transient network blip")
	})
	if err == nil {
		t.Fatalf("expected error after exhausting attempts, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts on a transient error, got %d", calls)
	}
}
