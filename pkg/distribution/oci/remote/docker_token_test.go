package remote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
)

func TestTrustedDockerTokenEndpoint(t *testing.T) {
	for _, registry := range []string{"docker.io", "index.docker.io", "registry-1.docker.io"} {
		for _, endpoint := range []string{"https://auth.docker.io/token", "https://AUTH.DOCKER.IO:443/token?service=registry.docker.io"} {
			u, err := url.Parse(endpoint)
			if err != nil {
				t.Fatal(err)
			}
			if !isTrustedDockerTokenEndpoint(u, registry) {
				t.Errorf("trusted Docker endpoint rejected: registry=%s endpoint=%s", registry, endpoint)
			}
		}
	}
	for _, endpoint := range []string{
		"http://auth.docker.io/token",
		"https://auth.docker.io:444/token",
		"https://auth.docker.io.evil.example/token",
		"https://evil-auth.docker.io/token",
		"https://auth.docker.io./token",
		"https://user@auth.docker.io/token",
		"https://auth.docker.io/admin",
		"https://auth.docker.io/%74oken",
		"https://auth.docker.io/token#fragment",
		"https://127.0.0.1/token",
	} {
		u, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if isTrustedDockerTokenEndpoint(u, "registry-1.docker.io") {
			t.Errorf("untrusted endpoint accepted: %s", endpoint)
		}
	}
	u, _ := url.Parse("https://auth.docker.io/token")
	for _, registry := range []string{"", "evil.example", "registry-1.docker.io.evil.example", "registry-1.docker.io:444"} {
		if isTrustedDockerTokenEndpoint(u, registry) {
			t.Errorf("Docker exception applied to unrelated registry: %s", registry)
		}
	}
}

// dockerTokenProxy serves a TLS-verified Docker token endpoint behind a local
// CONNECT proxy. Its custom dialer models Desktop's supplied proxy transport.
func dockerTokenProxy(t *testing.T, handler http.Handler) *http.Transport {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"auth.docker.io", "registry-1.docker.io", "index.docker.io"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	endpoint := httptest.NewUnstartedServer(handler)
	endpoint.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}}, MinVersion: tls.VersionTLS12}
	endpoint.StartTLS()
	t.Cleanup(endpoint.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusBadRequest)
			return
		}
		upstream, err := (&net.Dialer{}).DialContext(r.Context(), "tcp", endpoint.Listener.Addr().String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		fmt.Fprint(buffered, "HTTP/1.1 200 Connection Established\r\n\r\n")
		buffered.Flush()
		done := make(chan struct{})
		go func() {
			io.Copy(client, upstream)
			client.Close()
			close(done)
		}()
		io.Copy(upstream, buffered)
		upstream.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: "desktop-proxy:80"})
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "desktop-proxy:80" {
			return nil, fmt.Errorf("supplied dialer expected proxy address, got %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, proxy.Listener.Addr().String())
	}
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	t.Cleanup(transport.CloseIdleConnections)
	return transport
}

func TestDockerTokenWithoutLocalDNS(t *testing.T) {
	var lookups atomic.Int32
	originalResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		lookups.Add(1)
		return nil, fmt.Errorf("public DNS unavailable")
	}}
	t.Cleanup(func() { net.DefaultResolver = originalResolver })
	var hits atomic.Int32
	transport := dockerTokenProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Host != "auth.docker.io" || r.URL.Path != "/token" {
			http.Error(w, "unexpected token endpoint", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"via-proxy"}`)
	}))
	client := newGuardedAuthClient(transport, "registry-1.docker.io")
	guard := client.Transport.(*guardedAuthTransport)
	t.Cleanup(guard.proxied.CloseIdleConnections)
	t.Cleanup(guard.direct.CloseIdleConnections)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://auth.docker.io/token", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("guarded Docker token request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token request status: %d", resp.StatusCode)
	}
	ref, err := reference.ParseReference("ai/gemma3:4b")
	if err != nil {
		t.Fatal(err)
	}
	token, err := Exchange(t.Context(), ref.Context().Registry, nil, transport, nil, &PingResponse{
		WWWAuthenticate: WWWAuthenticate{Realm: "https://auth.docker.io/token"},
	})
	if err != nil || token.Token != "via-proxy" {
		t.Fatalf("Exchange failed: token=%v error=%v", token, err)
	}
	if hits.Load() != 2 || lookups.Load() != 0 {
		t.Fatalf("expected two proxied requests without DNS, got hits=%d DNS=%d", hits.Load(), lookups.Load())
	}
	// Unrelated registries still fail closed, even for the Docker token URL.
	otherClient := newGuardedAuthClient(transport, "evil.example")
	_, err = otherClient.Do(req)
	if err == nil || !strings.Contains(err.Error(), "resolving realm hostname") {
		t.Fatalf("untrusted registry should require local DNS: %v", err)
	}
	// Without a proxy, the validating dialer still requires local DNS.
	directTransport := transport.Clone()
	directTransport.Proxy = nil
	_, err = newGuardedAuthClient(directTransport, "registry-1.docker.io").Do(req)
	if err == nil || !strings.Contains(err.Error(), "resolving realm hostname") {
		t.Fatalf("direct connection should require validated DNS: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("blocked requests reached proxy: hits=%d", hits.Load())
	}
}

func TestDockerTokenRedirectToPrivateRealmBlocked(t *testing.T) {
	var hits atomic.Int32
	transport := dockerTokenProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "https://169.254.169.254/token", http.StatusFound)
	}))
	client := newGuardedAuthClient(transport, "registry-1.docker.io")
	guard := client.Transport.(*guardedAuthTransport)
	t.Cleanup(guard.proxied.CloseIdleConnections)
	t.Cleanup(guard.direct.CloseIdleConnections)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://auth.docker.io/token", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "disallowed IP address") {
		t.Fatalf("redirect to private realm should be rejected: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("redirect reached proxy endpoint: hits=%d", hits.Load())
	}
}
