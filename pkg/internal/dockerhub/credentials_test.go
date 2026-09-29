package dockerhub

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// authenticatedRegistry is a Docker Registry v2 handler that requires HTTP Basic
// authentication, mirroring how a JFrog Artifactory / Nexus / Harbor pull-through
// mirror behaves: unauthenticated requests get a 401 plus a WWW-Authenticate
// challenge, and only requests carrying the expected credentials are served.
type authenticatedRegistry struct {
	tag      string
	digest   string
	user     string
	password string

	// unauthorized counts the requests rejected for missing/incorrect credentials.
	unauthorized atomic.Int64
	// authorized counts the requests that presented the expected credentials.
	authorized atomic.Int64
}

func (h *authenticatedRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, password, ok := r.BasicAuth()
	if !ok || user != h.user || password != h.password {
		h.unauthorized.Add(1)
		// A Basic challenge keeps the test focused on credential plumbing rather
		// than on the token-fetch dance a Bearer challenge would trigger.
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	h.authorized.Add(1)

	switch {
	case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
	case strings.HasSuffix(r.URL.Path, "/manifests/"+h.tag):
		body := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
		w.Header().Set("Docker-Content-Digest", h.digest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
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

const (
	testMirrorUser     = "mirroruser"
	testMirrorPassword = "mirrorsecret"
	testMirrorTag      = "latest-cuda"
	testMirrorDigest   = "sha256:aa3e239c00000000000000000000000000000000000000000000000000c0ffee"
	testHubRef         = "registry-1.docker.io/docker/docker-model-backend-llamacpp:" + testMirrorTag
)

// isolateDockerConfig points credential lookups at an empty temporary home so a
// developer's real ~/.docker/config.json cannot influence the result.
func isolateDockerConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("DOCKER_HUB_USER", "")
	t.Setenv("DOCKER_HUB_PASSWORD", "")
	t.Setenv("DOCKER_USERNAME", "")
	t.Setenv("DOCKER_PASSWORD", "")
	if err := os.MkdirAll(filepath.Join(home, ".docker"), 0o755); err != nil {
		t.Fatalf("creating .docker dir: %v", err)
	}
	return home
}

// TestResolveDigest_AuthenticatedMirror_InjectedCredentials covers the path
// Docker Desktop uses: it holds registry credentials in process and injects a
// resolver, so backend image pulls authenticate against a private mirror without
// shelling out to a docker-credential-* helper.
func TestResolveDigest_AuthenticatedMirror_InjectedCredentials(t *testing.T) {
	isolateDockerConfig(t)

	registry := &authenticatedRegistry{
		tag: testMirrorTag, digest: testMirrorDigest,
		user: testMirrorUser, password: testMirrorPassword,
	}
	srv := httptest.NewServer(registry)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	var askedFor []string
	creds := func(host string) (string, string, error) {
		askedFor = append(askedFor, host)
		return testMirrorUser, testMirrorPassword, nil
	}

	got, err := ResolveDigest(ctx, testHubRef, []string{srv.URL}, creds)
	if err != nil {
		t.Fatalf("ResolveDigest with injected credentials failed: %v", err)
	}
	if got != testMirrorDigest {
		t.Fatalf("digest mismatch: got %q want %q", got, testMirrorDigest)
	}
	if registry.authorized.Load() == 0 {
		t.Fatal("expected at least one authenticated request to the mirror, got none")
	}

	// The credentials callback must be asked for the mirror's host, not
	// registry-1.docker.io: a mirror needs its own credentials.
	mirrorHost := strings.TrimPrefix(srv.URL, "http://")
	if len(askedFor) == 0 {
		t.Fatal("credentials callback was never invoked")
	}
	for _, host := range askedFor {
		if host != mirrorHost {
			t.Fatalf("credentials requested for %q, want the mirror host %q", host, mirrorHost)
		}
	}
}

// TestAuthenticatedRegistry_EnforcesAuthentication is the negative control for the
// tests that resolve through an authenticated mirror: it proves the fixture really
// does require credentials, so those tests cannot pass against a registry that
// serves everyone.
//
// It probes the fixture directly rather than going through ResolveDigest. Driving
// the resolver without credentials is not a usable assertion here: containerd
// treats the mirror as one host in an ordered list and falls through to
// registry-1.docker.io when it is rejected, so the call reaches the real Docker Hub
// — which makes the test depend on the network and leaves idle connections behind.
//
// That fall-through is worth naming, because it is why this class of bug is hard to
// see: when a mirror turns the fetcher away, the error that surfaces names Hub, not
// the mirror that actually refused it.
func TestAuthenticatedRegistry_EnforcesAuthentication(t *testing.T) {
	registry := &authenticatedRegistry{
		tag: testMirrorTag, digest: testMirrorDigest,
		user: testMirrorUser, password: testMirrorPassword,
	}
	srv := httptest.NewServer(registry)
	defer srv.Close()

	client := srv.Client()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v2/", http.NoBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("unauthenticated probe failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated probe returned %d, want 401", response.StatusCode)
	}
	if got := response.Header.Get("WWW-Authenticate"); got == "" {
		t.Fatal("401 response carried no WWW-Authenticate challenge")
	}

	request, err = http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v2/", http.NoBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.SetBasicAuth(testMirrorUser, testMirrorPassword)
	response, err = client.Do(request)
	if err != nil {
		t.Fatalf("authenticated probe failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated probe returned %d, want 200", response.StatusCode)
	}
}

// TestResolveDigest_AuthenticatedMirror_CredentialStore is the regression test for
// the bug this fix addresses. `docker login` on Docker Desktop stores the secret in
// the OS keychain via credsStore and leaves the auths entry's "auth" field empty, so
// a config.json-only lookup finds nothing, the mirror answers 401, and the resolver
// falls through to registry-1.docker.io — surfacing a Hub error that hides the real
// cause. Credentials must be read through the configured helper.
func TestResolveDigest_AuthenticatedMirror_CredentialStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake credential helper is a shell script")
	}
	home := isolateDockerConfig(t)

	registry := &authenticatedRegistry{
		tag: testMirrorTag, digest: testMirrorDigest,
		user: testMirrorUser, password: testMirrorPassword,
	}
	srv := httptest.NewServer(registry)
	defer srv.Close()
	mirrorHost := strings.TrimPrefix(srv.URL, "http://")

	// A credsStore entry with an auths entry whose "auth" field is empty is
	// exactly the state `docker login` leaves behind on Docker Desktop.
	config := fmt.Sprintf(`{"auths":{%q:{}},"credsStore":%q}`, mirrorHost, "modelrunnertest")
	if err := os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("writing config.json: %v", err)
	}

	// Stand in for docker-credential-desktop: read the server address on stdin
	// and print the credentials the keychain would hold.
	helperDir := t.TempDir()
	helper := fmt.Sprintf(`#!/bin/sh
[ "$1" = "get" ] || exit 1
cat >/dev/null
printf '{"ServerURL":"%s","Username":"%s","Secret":"%s"}\n'
`, mirrorHost, testMirrorUser, testMirrorPassword)
	helperPath := filepath.Join(helperDir, "docker-credential-modelrunnertest")
	if err := os.WriteFile(helperPath, []byte(helper), 0o700); err != nil {
		t.Fatalf("writing credential helper: %v", err)
	}
	t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	got, err := ResolveDigest(ctx, testHubRef, []string{srv.URL}, nil)
	if err != nil {
		t.Fatalf("ResolveDigest did not use the credential store: %v", err)
	}
	if got != testMirrorDigest {
		t.Fatalf("digest mismatch: got %q want %q", got, testMirrorDigest)
	}
	if registry.authorized.Load() == 0 {
		t.Fatal("expected at least one authenticated request to the mirror, got none")
	}
}

// TestDefaultCredentials_HubEnvIsNotOfferedToMirrors verifies that Docker Hub
// credentials taken from the environment are scoped to Docker Hub's own hosts.
// Returning them for any host handed them to whatever third-party registry mirror
// happened to be configured.
func TestDefaultCredentials_HubEnvIsNotOfferedToMirrors(t *testing.T) {
	isolateDockerConfig(t)
	t.Setenv("DOCKER_HUB_USER", "hubuser")
	t.Setenv("DOCKER_HUB_PASSWORD", "hubsecret")

	for _, host := range []string{"registry-1.docker.io", "docker.io", "index.docker.io"} {
		user, password, err := defaultCredentials(host)
		if err != nil {
			t.Fatalf("defaultCredentials(%q) returned error: %v", host, err)
		}
		if user != "hubuser" || password != "hubsecret" {
			t.Fatalf("defaultCredentials(%q) = (%q, %q), want the Hub credentials", host, user, password)
		}
	}

	for _, host := range []string{"devopsartifactory.corp.example.com", "nexus.internal:8082", "ghcr.io"} {
		user, password, err := defaultCredentials(host)
		if err != nil {
			t.Fatalf("defaultCredentials(%q) returned error: %v", host, err)
		}
		if user != "" || password != "" {
			t.Fatalf("defaultCredentials(%q) leaked Hub credentials: (%q, %q)", host, user, password)
		}
	}
}

// TestDefaultCredentials_InlineAuths covers the plain config.json case, including
// the Docker Hub key normalization: credentials for registry-1.docker.io are
// stored under "https://index.docker.io/v1/", which an exact host comparison
// never matched.
func TestDefaultCredentials_InlineAuths(t *testing.T) {
	home := isolateDockerConfig(t)

	encoded := base64.StdEncoding.EncodeToString([]byte("hubuser:hubsecret"))
	config := fmt.Sprintf(`{"auths":{"https://index.docker.io/v1/":{"auth":%q}}}`, encoded)
	if err := os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("writing config.json: %v", err)
	}

	user, password, err := defaultCredentials("registry-1.docker.io")
	if err != nil {
		t.Fatalf("defaultCredentials returned error: %v", err)
	}
	if user != "hubuser" || password != "hubsecret" {
		t.Fatalf("defaultCredentials = (%q, %q), want (hubuser, hubsecret)", user, password)
	}
}
