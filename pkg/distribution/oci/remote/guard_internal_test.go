package remote

import (
	"net/http"
	"net/http/httptest"
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
