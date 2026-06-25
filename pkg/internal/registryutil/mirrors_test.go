package registryutil

import (
	"testing"

	"github.com/containerd/containerd/v2/core/remotes/docker"
)

// hostsForDockerHub resolves the mirror hosts that apply to a Docker Hub
// reference. It fails the test if RegistryHosts returns an error.
func hostsForDockerHub(t *testing.T, mirrors []string) []docker.RegistryHost {
	t.Helper()
	authorizer := docker.NewDockerAuthorizer()
	hosts, err := RegistryHosts(mirrors, authorizer, nil)("registry-1.docker.io")
	if err != nil {
		t.Fatalf("RegistryHosts returned error: %v", err)
	}
	return hosts
}

// TestRegistryHosts_PreservesMirrorPath verifies that a path prefix on the
// mirror URL (e.g. a JFrog Artifactory repository path) is preserved and the
// Registry v2 API root is appended to it, rather than the request being sent to
// the host root. This is the path enterprise customers behind an Artifactory
// mirror need.
func TestRegistryHosts_PreservesMirrorPath(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mirror     string
		wantHost   string
		wantScheme string
		wantPath   string
	}{
		{
			name:       "path prefix preserved",
			mirror:     "https://devopsartifactory.corp.lpl.com/artifactory/docker",
			wantHost:   "devopsartifactory.corp.lpl.com",
			wantScheme: "https",
			wantPath:   "/artifactory/docker/v2",
		},
		{
			name:       "trailing slash trimmed",
			mirror:     "https://mirror.example.com/artifactory/docker/",
			wantHost:   "mirror.example.com",
			wantScheme: "https",
			wantPath:   "/artifactory/docker/v2",
		},
		{
			name:       "no path uses v2 root",
			mirror:     "https://mirror.example.com",
			wantHost:   "mirror.example.com",
			wantScheme: "https",
			wantPath:   "/v2",
		},
		{
			name:       "no scheme with path",
			mirror:     "mirror.example.com/artifactory/docker",
			wantHost:   "mirror.example.com",
			wantScheme: "https",
			wantPath:   "/artifactory/docker/v2",
		},
		{
			name:       "no scheme bare host",
			mirror:     "mirror.example.com:5000",
			wantHost:   "mirror.example.com:5000",
			wantScheme: "https",
			wantPath:   "/v2",
		},
		{
			name:       "http scheme preserved",
			mirror:     "http://mirror.internal/proxy/docker",
			wantHost:   "mirror.internal",
			wantScheme: "http",
			wantPath:   "/proxy/docker/v2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hosts := hostsForDockerHub(t, []string{tc.mirror})
			if len(hosts) == 0 {
				t.Fatalf("expected at least the mirror host, got none")
			}
			// The mirror is always tried first, before the upstream fallback.
			mirror := hosts[0]
			if mirror.Host != tc.wantHost {
				t.Errorf("Host: got %q want %q", mirror.Host, tc.wantHost)
			}
			if mirror.Scheme != tc.wantScheme {
				t.Errorf("Scheme: got %q want %q", mirror.Scheme, tc.wantScheme)
			}
			if mirror.Path != tc.wantPath {
				t.Errorf("Path: got %q want %q", mirror.Path, tc.wantPath)
			}
		})
	}
}

// TestRegistryHosts_AppendsUpstreamFallback verifies the upstream registry is
// still appended after the mirror, so a mirror miss falls through to
// registry-1.docker.io.
func TestRegistryHosts_AppendsUpstreamFallback(t *testing.T) {
	hosts := hostsForDockerHub(t, []string{"https://mirror.example.com/artifactory/docker"})
	if len(hosts) < 2 {
		t.Fatalf("expected mirror + upstream, got %d host(s)", len(hosts))
	}
	last := hosts[len(hosts)-1]
	if last.Host != "registry-1.docker.io" {
		t.Errorf("expected upstream fallback registry-1.docker.io, got %q", last.Host)
	}
}

// TestRegistryHosts_NonDockerHubBypassesMirror verifies mirrors are only applied
// to Docker Hub references; other registries use the default configuration.
func TestRegistryHosts_NonDockerHubBypassesMirror(t *testing.T) {
	authorizer := docker.NewDockerAuthorizer()
	hosts, err := RegistryHosts([]string{"https://mirror.example.com/artifactory/docker"}, authorizer, nil)("ghcr.io")
	if err != nil {
		t.Fatalf("RegistryHosts returned error: %v", err)
	}
	for _, h := range hosts {
		if h.Host == "mirror.example.com" {
			t.Fatalf("mirror should not be applied to non-Docker-Hub host ghcr.io")
		}
	}
}
