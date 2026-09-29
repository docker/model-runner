package remote

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestNewGuardedAuthClientBlocksLoopback(t *testing.T) {
	var hits atomic.Int32
	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer internalService.Close()

	client := newGuardedAuthClient(nil)
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

	client := newGuardedAuthClient(base)

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
