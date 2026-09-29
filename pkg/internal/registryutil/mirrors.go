package registryutil

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/containerd/containerd/v2/core/remotes/docker"
)

// RegistryHosts returns a RegistryHosts function that tries mirrors before the upstream registry.
// Mirrors are only applied for Docker Hub (registry-1.docker.io / docker.io); all other
// registries use the default configuration. When mirrors is empty the default configuration
// is returned unchanged.
//
// client is used for both mirror and upstream connections; pass nil to use http.DefaultClient.
func RegistryHosts(mirrors []string, authorizer docker.Authorizer, client *http.Client) docker.RegistryHosts {
	var defaultOpts []docker.RegistryOpt
	defaultOpts = append(defaultOpts, docker.WithAuthorizer(authorizer))
	if client != nil {
		defaultOpts = append(defaultOpts, docker.WithClient(client))
	}
	defaults := docker.ConfigureDefaultRegistries(defaultOpts...)
	if len(mirrors) == 0 {
		return defaults
	}
	return func(host string) ([]docker.RegistryHost, error) {
		if host != "registry-1.docker.io" && host != "docker.io" {
			return defaults(host)
		}
		mirrorClient := client
		if mirrorClient == nil {
			mirrorClient = http.DefaultClient
		}
		var hosts []docker.RegistryHost
		for _, mirror := range mirrors {
			host, scheme, path, ok := parseMirror(mirror)
			if !ok {
				continue
			}
			hosts = append(hosts, docker.RegistryHost{
				Client:       mirrorClient,
				Authorizer:   authorizer,
				Host:         host,
				Scheme:       scheme,
				Path:         path,
				Capabilities: docker.HostCapabilityPull | docker.HostCapabilityResolve,
			})
		}
		upstream, err := defaults("registry-1.docker.io")
		if err != nil {
			return nil, err
		}
		return append(hosts, upstream...), nil
	}
}

// parseMirror normalizes a configured mirror string into the host, scheme and
// Registry v2 API base path used to build a docker.RegistryHost. It returns
// ok=false (after logging a warning) for a mirror that does not parse or has no
// host, so the caller can skip it.
//
// Normalization rules:
//   - A mirror without a scheme (e.g. "host:5000/path" or "127.0.0.1:5000")
//     defaults to https. The scheme is prepended before parsing because
//     url.Parse mishandles scheme-less inputs — an IP:port errors on the colon,
//     a host:port is parsed as scheme:opaque.
//   - Any path prefix is preserved (e.g. a JFrog Artifactory repository path
//     "/artifactory/api/docker/<repo>"); without this a mirror with a path would
//     be queried at the host root and fail.
//   - "/v2" is appended unless the configured path already ends with it, so the
//     suffix is never doubled.
func parseMirror(mirror string) (host, scheme, path string, ok bool) {
	raw := mirror
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		slog.Warn("skipping invalid registry mirror", "mirror", mirror, "error", err)
		return "", "", "", false
	}
	if u.Host == "" {
		slog.Warn("skipping invalid registry mirror", "mirror", mirror, "error", "empty host")
		return "", "", "", false
	}
	path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/v2") {
		path += "/v2"
	}
	return u.Host, u.Scheme, path, true
}
