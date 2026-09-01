package remote_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
)

// TestPullSSRF_RealmNotFollowedToInternalService exercises the pull path end to
// end: a malicious registry answers every request with a 401 Bearer challenge
// whose realm points at a *different* internal host (the cloud metadata service
// at 169.254.169.254). The token fetch that containerd's authorizer performs
// against that realm must be blocked, so the internal service is never
// contacted. This is the cross-host pivot the SSRF fix targets.
//
// A realm on the registry's OWN host is permitted (same trust domain), so
// internal/corporate registries keep working — see
// transport_test.go's TestExchangeAllowsInternalRealmOnSameHost.
func TestPullSSRF_RealmNotFollowedToInternalService(t *testing.T) {
	maliciousRegistry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer realm="http://169.254.169.254/latest/meta-data",service="evil-registry"`))
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
		t.Fatal("remote.Image should have failed: the token realm resolves to a different internal host (cloud metadata) and must be rejected")
	}
	if !strings.Contains(err.Error(), "realm") {
		t.Errorf("expected a realm-related rejection error, got: %q", err.Error())
	}
}
