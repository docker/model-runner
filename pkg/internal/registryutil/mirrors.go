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
			// A mirror may be given without a scheme (e.g. "host:5000/path" or
			// "127.0.0.1:5000"). url.Parse mishandles those — an IP:port errors
			// on the colon, a host:port is parsed as scheme:opaque — so default
			// to https before parsing when no scheme is present.
			raw := mirror
			if !strings.Contains(raw, "://") {
				raw = "https://" + raw
			}
			u, err := url.Parse(raw)
			if err != nil {
				slog.Warn("skipping invalid registry mirror", "mirror", mirror, "error", err)
				continue
			}
			if u.Host == "" {
				slog.Warn("skipping invalid registry mirror", "mirror", mirror, "error", "empty host")
				continue
			}
			// Preserve any path prefix on the mirror (e.g. a JFrog Artifactory
			// repository path "/artifactory/api/docker/<repo>") and append the
			// Registry v2 API root. Without this, a mirror configured with a path
			// would be queried at the host root and fail.
			path := strings.TrimRight(u.Path, "/") + "/v2"
			hosts = append(hosts, docker.RegistryHost{
				Client:       mirrorClient,
				Authorizer:   authorizer,
				Host:         u.Host,
				Scheme:       u.Scheme,
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
