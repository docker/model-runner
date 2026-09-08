package inference

// RegistryCredentials resolves registry credentials for a registry host,
// returning the username and secret to authenticate with. Returning an empty
// username and secret with a nil error means no credentials are available for
// that host and anonymous access should be attempted.
//
// An embedder that already holds registry credentials in process supplies one of
// these so backend image pulls authenticate without shelling out to a
// docker-credential-* helper — which also removes any dependency on the helper
// being on PATH. Docker Desktop does this: it is itself the credential backend.
//
// When nil, backends fall back to resolving credentials from the environment and
// ~/.docker/config.json (including credHelpers and credsStore).
//
// The host is the registry the request is being made to, so for a pull routed
// through a registry mirror it is the mirror's host, not registry-1.docker.io.
// Credentials must therefore be resolved per host: Docker Hub credentials do not
// apply to a third-party mirror.
type RegistryCredentials func(host string) (username, secret string, err error)
