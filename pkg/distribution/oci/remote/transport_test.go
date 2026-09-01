package remote_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
)

// emptyRegistry is a zero-value reference.Registry.
// reference.Registry is a concrete struct (not an interface), so nil cannot
// be used; the zero value is a safe placeholder when registry-specific
// behaviour is not exercised by the test.
var emptyRegistry reference.Registry

// pingResponseForRealm returns a *remote.PingResponse whose Realm field is
// set to the given URL.  This simulates the result of calling Ping() against
// a malicious registry that advertises an attacker-controlled realm.
func pingResponseForRealm(realm string) *remote.PingResponse {
	return &remote.PingResponse{
		WWWAuthenticate: remote.WWWAuthenticate{
			Realm:   realm,
			Service: "evil-registry",
			Scope:   "repository:evil/model:pull",
		},
	}
}

// TestExchangeSSRF_RequestSentToRealmURL verifies that prevents Exchange() from
// contacting internal services via a malicious realm URL.
func TestExchangeSSRF_RequestSentToRealmURL(t *testing.T) {
	var hitCount atomic.Int32

	// "Internal service" — simulates a host-local endpoint (127.0.0.1) that
	// should never be reachable via a registry token-exchange flow.
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"error":"not a token endpoint"}`)
	}))
	defer internalService.Close()

	// Build a PingResponse whose Realm points at the internal service —
	// this is what a malicious registry would return in its 401 response.
	pr := pingResponseForRealm(internalService.URL + "/internal/credentials")

	_, err := remote.Exchange(t.Context(), emptyRegistry, nil, nil, []string{"repository:x:pull"}, pr)
	if err == nil {
		t.Fatal("Exchange() should have returned an error when the realm URL resolves to a loopback address")
	}
	if hitCount.Load() > 0 {
		t.Errorf("SSRF not blocked: Exchange() sent %d request(s) to the internal service at %s — realm URL validation should have rejected 127.0.0.1 before making any HTTP request", hitCount.Load(), internalService.URL)
	}
	if !strings.Contains(err.Error(), "realm URL rejected") {
		t.Errorf("expected error to mention realm URL rejection, got: %q", err.Error())
	}
}

// TestExchangeAllowsInternalRealmOnSameHost verifies that an internal/corporate
// registry whose token endpoint lives on the same host (e.g. an RFC1918 address
// or loopback) is NOT blocked by the SSRF guard. The realm is within the same
// trust domain as the registry being pulled, so private/loopback/link-local
// addresses must be permitted there.
func TestExchangeAllowsInternalRealmOnSameHost(t *testing.T) {
	var hitCount atomic.Int32

	// "Internal service" on 127.0.0.1 — without the same-host exception this
	// would be rejected by the loopback blocklist.
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"token":"internal-token"}`)
	}))
	defer internalService.Close()

	// Registry host matches the realm host (both resolve to 127.0.0.1).
	ref, err := reference.ParseReference("127.0.0.1:5000/myimage")
	if err != nil {
		t.Fatalf("failed to parse reference: %v", err)
	}

	pr := pingResponseForRealm(internalService.URL + "/token")

	_, err = remote.Exchange(t.Context(), ref.Context().Registry, nil, nil, []string{"repository:x:pull"}, pr)
	if err != nil && strings.Contains(err.Error(), "realm URL rejected") {
		t.Fatalf("internal same-host realm was wrongly rejected: %v", err)
	}
	if hitCount.Load() == 0 {
		t.Errorf("expected the token request to reach the internal service at %s (same-host realms must be allowed)", internalService.URL)
	}
}

// TestExchangeSSRF_SensitiveBodyNotReflectedInError verifies that Exchange()
// does NOT include a token-endpoint response body in the error it returns to
// the caller.
func TestExchangeSSRF_SensitiveBodyNotReflectedInError(t *testing.T) {
	sensitiveData := map[string]string{
		"db_password":      "s3cret_example",
		"internal_api_key": "sk-example-key",
		"proof":            "THIS_DATA_READ_VIA_SSRF",
	}
	sensitiveJSON, err := json.Marshal(sensitiveData)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}

	// "Internal service" returns a non-200 response containing sensitive data.
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(sensitiveJSON) //nolint:errcheck
	}))
	defer internalService.Close()

	pr := pingResponseForRealm(internalService.URL + "/admin/api/keys")

	_, err = remote.Exchange(t.Context(), emptyRegistry, nil, nil, []string{"repository:x:pull"}, pr)
	if err == nil {
		t.Fatal("expected an error from Exchange() when realm URL is blocked or token endpoint returns 401, got nil")
	}

	errMsg := err.Error()

	// The error must indicate realm rejection, not a remote HTTP status code.
	if !strings.Contains(errMsg, "realm URL rejected") {
		t.Errorf("expected error to mention realm URL rejection (Fix 2a), got: %q", errMsg)
	}

	// the response body must never appear in the returned error
	for _, sensitive := range []string{"s3cret_example", "sk-example-key", "THIS_DATA_READ_VIA_SSRF"} {
		if strings.Contains(errMsg, sensitive) {
			t.Errorf("BODY REFLECTION vulnerability: error contains sensitive value %q — the response body must not be included in errors returned to the caller (log at debug level instead)", sensitive)
		}
	}
}
