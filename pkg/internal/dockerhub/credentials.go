package dockerhub

import (
	"log/slog"
	"os"

	"github.com/docker/model-runner/pkg/distribution/oci/authn"
)

// Credentials resolves registry credentials for a registry host, returning the
// username and secret to authenticate with. An empty username and secret with a
// nil error means no credentials are available and the caller should attempt
// anonymous access.
//
// It is an alias rather than a defined type so that callers can pass their own
// named function type (e.g. inference.RegistryCredentials) without a conversion.
type Credentials = func(host string) (username, secret string, err error)

// isHubHost reports whether host is one of the names Docker Hub is reached by,
// and therefore whether Docker Hub credentials apply to it. A registry mirror
// standing in for Hub is not a Hub host: it needs its own credentials, and
// offering Hub credentials to an unrelated third-party host would leak them.
func isHubHost(host string) bool {
	switch host {
	case "docker.io", "registry-1.docker.io", "index.docker.io":
		return true
	}
	return false
}

// defaultCredentials resolves registry credentials the way the Docker CLI does:
// DOCKER_HUB_USER/DOCKER_HUB_PASSWORD for Docker Hub itself, then
// ~/.docker/config.json — credential helpers (credHelpers), the credential store
// (credsStore), and finally inline auths entries.
//
// Consulting the credential store is what makes authenticated registry mirrors
// work. `docker login` on Docker Desktop stores the secret in the OS keychain and
// leaves the auths entry's "auth" field empty, so a config.json-only lookup finds
// nothing and the request is sent unauthenticated. Against a mirror that requires
// authentication that yields a 401, the resolver then falls through to
// registry-1.docker.io and the surfaced error names Hub rather than the mirror —
// which makes the real cause (missing credentials for the mirror) invisible.
//
// Docker Desktop supplies its own resolver instead of this one, because it holds
// the credentials in process and does not need to shell out to a helper. See
// inference.RegistryCredentials.
func defaultCredentials(host string) (string, string, error) {
	if isHubHost(host) {
		if user, password := os.Getenv("DOCKER_HUB_USER"), os.Getenv("DOCKER_HUB_PASSWORD"); user != "" && password != "" {
			slog.Debug("using Docker Hub credentials from the environment", "host", host, "user", user)
			return user, password, nil
		}
	}
	authenticator, err := authn.FromDockerConfig(host)
	if err != nil {
		return "", "", err
	}
	if authenticator == nil {
		slog.Debug("no registry credentials found", "host", host)
		return "", "", nil
	}
	config, err := authenticator.Authorization()
	if err != nil {
		return "", "", err
	}
	return credentialsFromAuthConfig(host, config)
}

// credentialsFromAuthConfig converts an authn.AuthConfig into the
// (username, secret) pair containerd's authorizer expects. containerd treats an
// empty username as "the secret is a token", which is how identity and registry
// tokens are conveyed.
func credentialsFromAuthConfig(host string, config *authn.AuthConfig) (string, string, error) {
	if config == nil {
		return "", "", nil
	}
	switch {
	case config.Username != "" && config.Password != "":
		slog.Debug("using registry credentials", "host", host, "user", config.Username)
		return config.Username, config.Password, nil
	case config.IdentityToken != "":
		slog.Debug("using identity token for registry", "host", host)
		return "", config.IdentityToken, nil
	case config.RegistryToken != "":
		slog.Debug("using registry token for registry", "host", host)
		return "", config.RegistryToken, nil
	}
	slog.Debug("no usable registry credentials", "host", host)
	return "", "", nil
}
