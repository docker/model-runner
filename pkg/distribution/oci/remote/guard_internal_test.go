package remote

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// parseRealm parses a realm URL for table-driven cases, failing the test on a
// malformed URL.
func parseRealm(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}

// newProxyTransport starts a local proxy that serves a token response and
// returns an http.Transport pointing at it, plus a counter of requests it
// received. Used by the proxied realm tests to assert both that a request is
// allowed and that a rejected realm never reaches the proxy.
func newProxyTransport(t *testing.T, token string) (*http.Transport, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"token\":%q}\n", token)
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parsing proxy URL: %v", err)
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = http.ProxyURL(proxyURL)
	return base, &hits
}

func TestNewGuardedAuthClientBlocksLoopback(t *testing.T) {
	var hits atomic.Int32
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer internalService.Close()

	client := newGuardedAuthClient(nil, "", "")
	resp, err := client.Get(internalService.URL) //nolint:noctx
	if err == nil {
		resp.Body.Close()
		t.Fatal("guarded auth client should refuse to connect to a loopback token endpoint")
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("guarded auth client contacted the loopback service %d time(s); the dialer must reject it before connecting", got)
	}
}

// TestGuardedAuthClientHonorsProxyOnPrivateAddress reproduces the proxied
// deployment regression: production transports carry Proxy settings
// (server.go sets http.ProxyFromEnvironment), and proxies commonly live on
// private or loopback addresses. The guard must validate the realm host, not
// the proxy's address — otherwise every token fetch behind a proxy fails and
// model pulls break.
func TestGuardedAuthClientHonorsProxyOnPrivateAddress(t *testing.T) {
	var proxiedURLs []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedURLs = append(proxiedURLs, r.RequestURI)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"via-proxy"}`)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parsing proxy URL: %v", err)
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = http.ProxyURL(proxyURL)

	client := newGuardedAuthClient(base, "", "")

	// A public realm must be reachable through the loopback proxy. 203.0.113.0/24
	// is TEST-NET-3: never routable, so a hit proves the request went via the proxy.
	resp, err := client.Get("http://203.0.113.10/token") //nolint:noctx
	if err != nil {
		t.Fatalf("token fetch through a loopback proxy should succeed, got: %v", err)
	}
	resp.Body.Close()
	if len(proxiedURLs) != 1 || proxiedURLs[0] != "http://203.0.113.10/token" {
		t.Errorf("expected exactly the realm request to go through the proxy, got %v", proxiedURLs)
	}

	// A disallowed realm must still be rejected before reaching the proxy.
	resp, err = client.Get("http://169.254.169.254/token") //nolint:noctx
	if err == nil {
		resp.Body.Close()
		t.Fatal("a link-local realm must be rejected even when a proxy is configured")
	}
	if len(proxiedURLs) != 1 {
		t.Errorf("the disallowed realm request must not reach the proxy, got %v", proxiedURLs)
	}
}

func TestResolveAndValidateHost(t *testing.T) {
	disallowed := []string{
		"127.0.0.1",
		"10.1.2.3",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"::1",
		"localhost",
		"host.docker.internal",
		"model-runner.docker.internal",
		"gateway.docker.internal",
	}
	for _, host := range disallowed {
		if _, err := resolveAndValidateHost(host, "443"); err == nil {
			t.Errorf("resolveAndValidateHost(%q) = nil error; want rejection", host)
		}
	}

	// A public literal IP carries no DNS to rebind and must be accepted.
	if _, err := resolveAndValidateHost("8.8.8.8", "443"); err != nil {
		t.Errorf("resolveAndValidateHost(%q) = %v; want nil error", "8.8.8.8", err)
	}
}

// TestNormalizeAuthority covers the canonical authority forms the trust-domain
// check compares. Bracketed IPv6 literals without a port must not be
// double-bracketed, or they would never match the dialer's "[::1]:80" form.
func TestNormalizeAuthority(t *testing.T) {
	cases := []struct {
		host, scheme, want string
	}{
		{"registry.example", "https", "registry.example:443"},
		{"registry.example", "http", "registry.example:80"},
		{"registry.example:5000", "https", "registry.example:5000"},
		{"[::1]", "http", "[::1]:80"},
		{"[::1]", "https", "[::1]:443"},
		{"[::1]:5000", "https", "[::1]:5000"},
		{"::1", "http", "[::1]:80"},
		{"", "https", ""},
	}
	for _, tc := range cases {
		if got := normalizeAuthority(tc.host, tc.scheme); got != tc.want {
			t.Errorf("normalizeAuthority(%q, %q) = %q; want %q", tc.host, tc.scheme, got, tc.want)
		}
	}
}

// TestIsSameAuthorityRealm verifies that only the single authority actually
// contacted is trusted: a different port, or the other scheme's default port
// on the same hostname, is a different service and must not be exempted.
func TestIsSameAuthorityRealm(t *testing.T) {
	trusted := []struct{ realm, host, scheme string }{
		{"https://registry.example/token", "registry.example", "https"},
		{"https://registry.example:443/token", "registry.example", "https"},
		{"http://registry.example/token", "registry.example", "http"},
		{"https://registry.example:5000/token", "registry.example:5000", "https"},
		{"http://[::1]:5000/token", "[::1]:5000", "http"},
	}
	for _, tc := range trusted {
		if !isSameAuthorityRealm(parseRealm(t, tc.realm), tc.host, tc.scheme) {
			t.Errorf("isSameAuthorityRealm(%q, %q, %q) = false; want true", tc.realm, tc.host, tc.scheme)
		}
	}

	distrusted := []struct{ realm, host, scheme string }{
		// HTTPS registry must not implicitly trust the port-80 service.
		{"http://registry.example/token", "registry.example", "https"},
		// Same IP, different port: a different service.
		{"https://registry.example:5001/token", "registry.example:5000", "https"},
		// A different host entirely.
		{"https://metadata.internal/token", "registry.example", "https"},
		// No registry host known: trust nothing.
		{"https://registry.example/token", "", "https"},
	}
	for _, tc := range distrusted {
		if isSameAuthorityRealm(parseRealm(t, tc.realm), tc.host, tc.scheme) {
			t.Errorf("isSameAuthorityRealm(%q, %q, %q) = true; want false", tc.realm, tc.host, tc.scheme)
		}
	}
}

// TestIsMetadataHostname checks the name match directly, without DNS, so it
// does not depend on whether the metadata hostnames resolve where tests run.
func TestIsMetadataHostname(t *testing.T) {
	for _, name := range []string{
		"metadata.google.internal",
		"metadata.google.internal.",
		"METADATA.Google.Internal.",
		"metadata.goog",
		"metadata.goog.",
	} {
		if !isMetadataHostname(name) {
			t.Errorf("isMetadataHostname(%q) = false; want true", name)
		}
	}
	for _, name := range []string{"", ".", "registry.example", "metadata.example", "notmetadata.goog"} {
		if isMetadataHostname(name) {
			t.Errorf("isMetadataHostname(%q) = true; want false", name)
		}
	}
}

// TestMetadataAddressesAlwaysBlocked verifies the narrow safety net from #1050:
// cloud metadata endpoints must never be contacted as a token realm, even when
// the realm is on the registry's own authority (the trust-domain exception).
func TestMetadataAddressesAlwaysBlocked(t *testing.T) {
	for _, tc := range []struct{ realm, host, scheme string }{
		// Literal metadata addresses advertised as the registry's own realm.
		{"https://169.254.169.254/token", "169.254.169.254", "https"},
		{"http://169.254.170.2/token", "169.254.170.2:80", "http"},
		{"https://[fd00:ec2::254]/token", "[fd00:ec2::254]", "https"},
		{"http://100.100.100.200/token", "100.100.100.200:80", "http"},
		// A metadata hostname must be rejected regardless of its resolution.
		{"https://metadata.google.internal/token", "metadata.google.internal", "https"},
		// The trailing-dot (absolute DNS) spelling names the same service.
		{"https://metadata.google.internal./token", "metadata.google.internal.", "https"},
		{"https://metadata.goog./token", "metadata.goog.", "https"},
		{"https://METADATA.GOOGLE.INTERNAL./token", "evil.example", "https"},
		// An unrelated realm pointing at metadata must also be rejected.
		{"https://169.254.169.254/token", "evil.example", "https"},
	} {
		if err := validateTokenEndpointURL(parseRealm(t, tc.realm), tc.host, tc.scheme); err == nil {
			t.Errorf("validateTokenEndpointURL(%q, %q, %q) = nil; want metadata rejection", tc.realm, tc.host, tc.scheme)
		}
	}

	// The direct dialer's same-authority path must enforce the same rule.
	for _, addr := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254"} {
		if _, err := resolveHost(addr, "443"); err == nil {
			t.Errorf("resolveHost(%q) = nil; want metadata rejection", addr)
		}
	}
	// Other private addresses remain allowed for same-authority realms.
	for _, addr := range []string{"10.1.2.3", "192.168.1.1", "127.0.0.1", "169.254.1.49", "::1"} {
		if _, err := resolveHost(addr, "443"); err != nil {
			t.Errorf("resolveHost(%q) = %v; want nil error (internal registry)", addr, err)
		}
	}
}

// TestProxiedSameAuthorityRealmValidatesDestination reproduces the finding that
// a proxied same-authority realm used to be forwarded without any destination
// validation. A realm that resolves to a metadata address must be rejected
// before the request reaches the proxy.
func TestProxiedSameAuthorityRealmValidatesDestination(t *testing.T) {
	base, hits := newProxyTransport(t, "should-not-be-fetched")

	// The realm is on the registry's own authority, but its destination is a
	// metadata address, so it must be rejected before reaching the proxy.
	client := newGuardedAuthClient(base, "169.254.169.254", "https")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://169.254.169.254/token", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(req); err == nil {
		t.Fatal("proxied same-authority realm resolving to metadata must be rejected")
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("proxy received %d request(s); metadata realm must be rejected before reaching it", got)
	}
}

// TestSameAuthorityPrivateRealmStillWorks guards the actual motivation of the
// change: an internal registry on a private address, whose token endpoint is on
// its own authority, must remain reachable through a proxy.
func TestSameAuthorityPrivateRealmStillWorks(t *testing.T) {
	base, hits := newProxyTransport(t, "internal-token")

	// 10.0.0.5 is private but not a metadata address.
	client := newGuardedAuthClient(base, "10.0.0.5:5000", "http")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://10.0.0.5:5000/token", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	if err != nil && strings.Contains(err.Error(), "realm URL rejected") {
		t.Fatalf("internal same-authority realm must not be rejected: %v", err)
	}
	if got := hits.Load(); got == 0 {
		t.Error("internal same-authority realm never reached the proxy")
	}
}
