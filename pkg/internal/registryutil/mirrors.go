package registryutil

import (
	"log/slog"
	"net/http"
	"net/url"

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
			u, err := url.Parse(mirror)
			if err != nil {
				slog.Warn("skipping invalid registry mirror", "mirror", mirror, "error", err)
				continue
			}
			mirrorHost := u.Host
			scheme := u.Scheme
			if mirrorHost == "" {
				mirrorHost = mirror
				scheme = "https"
			}
			hosts = append(hosts, docker.RegistryHost{
				Client:       mirrorClient,
				Authorizer:   authorizer,
				Host:         mirrorHost,
				Scheme:       scheme,
				Path:         "/v2",
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
