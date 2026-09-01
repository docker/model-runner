package remote_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
)

// TestPullSSRF_RealmNotFollowedToInternalService exercises the pull path end to
// end: a malicious registry answers every request with a 401 Bearer challenge
// whose realm points at a loopback "internal service". The token fetch that
// containerd's authorizer performs against that realm must be blocked, so the
// internal service is never contacted.
//
// The internal service sits on the same IP as the registry but a different
// port: trust is scoped to the exact authority (host and port) being contacted,
// so advertising another service on the same host must not be enough to have
// the token request followed there.
func TestPullSSRF_RealmNotFollowedToInternalService(t *testing.T) {
	var internalHits atomic.Int32
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"leaked-via-ssrf"}`)
	}))
	defer internalService.Close()

	maliciousRegistry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate",
			`Bearer realm="`+internalService.URL+`/token",service="evil-registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer maliciousRegistry.Close()

	registryHost := strings.TrimPrefix(maliciousRegistry.URL, "http://")
	ref, err := reference.ParseReference(registryHost + "/evil/model:latest")
	if err != nil {
		t.Fatalf("parsing reference: %v", err)
	}

	_, err = remote.Image(ref, remote.WithContext(t.Context()), remote.WithPlainHTTP(true))
	if err == nil {
		t.Fatal("remote.Image should have failed: the token realm points at a different authority on the same host and must be rejected")
	}
	if hits := internalHits.Load(); hits != 0 {
		t.Errorf("SSRF not blocked on the pull path: the internal service at %s was contacted %d time(s) via the token realm", internalService.URL, hits)
	}
}
