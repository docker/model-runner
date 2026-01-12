// Package authn provides authentication support for registry operations.
// This replaces go-containerregistry's authn package.
package authn

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	credHelperClient "github.com/docker/docker-credential-helpers/client"
	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
)

// Authenticator provides authentication credentials for registry operations.
type Authenticator interface {
	// Authorization returns the authentication credentials.
	Authorization() (*AuthConfig, error)
}

// AuthConfig contains authentication credentials.
type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// Basic implements Authenticator for basic username/password authentication.
type Basic struct {
	Username string
	Password string
}

// Authorization returns the basic auth credentials.
func (b *Basic) Authorization() (*AuthConfig, error) {
	return &AuthConfig{
		Username: b.Username,
		Password: b.Password,
	}, nil
}

// Bearer implements Authenticator for bearer token authentication.
type Bearer struct {
	Token string
}

// NewBearer creates a new Bearer authenticator.
func NewBearer(token string) *Bearer {
	return &Bearer{Token: token}
}

// Authorization returns the bearer token credentials.
func (b *Bearer) Authorization() (*AuthConfig, error) {
	return &AuthConfig{
		RegistryToken: b.Token,
	}, nil
}

// Anonymous implements Authenticator for anonymous access.
type Anonymous struct{}

// Authorization returns empty credentials for anonymous access.
func (a *Anonymous) Authorization() (*AuthConfig, error) {
	return &AuthConfig{}, nil
}

// Resource represents a registry resource that can be resolved for authentication.
type Resource interface {
	// RegistryStr returns the registry hostname.
	RegistryStr() string
}

// Keychain provides a way to resolve credentials for registries.
type Keychain interface {
	// Resolve returns an Authenticator for the given resource.
	Resolve(Resource) (Authenticator, error)
}

// defaultKeychain implements Keychain using the Docker config file.
type defaultKeychain struct{}

// DefaultKeychain is the default keychain that reads from ~/.docker/config.json.
var DefaultKeychain Keychain = &defaultKeychain{}

// Resolve returns credentials for the given resource from the Docker config file.
func (k *defaultKeychain) Resolve(r Resource) (Authenticator, error) {
	registry := r.RegistryStr()

	// Try environment variables first
	if username := os.Getenv("DOCKER_HUB_USER"); username != "" {
		if password := os.Getenv("DOCKER_HUB_PASSWORD"); password != "" {
			return &Basic{
				Username: username,
				Password: password,
			}, nil
		}
	}

	// Read from Docker config file
	auth, err := getAuthFromConfig(registry)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Anonymous{}, nil
		}
		return nil, err
	}
	if auth != nil {
		return auth, nil
	}

	return &Anonymous{}, nil
}

// dockerConfig represents the structure of ~/.docker/config.json
type dockerConfig struct {
	Auths       map[string]AuthConfig `json:"auths"`
	CredsStore  string                `json:"credsStore,omitempty"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
}

// getAuthFromConfig reads authentication from the Docker config file.
func getAuthFromConfig(registry string) (Authenticator, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".docker", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Determine the server address to look up credentials for
	// For Docker Hub, we need to use the full URL format
	serverAddress := getServerAddressForRegistry(registry)

	// Try credential helpers first (credHelpers for specific registries)
	if helper, ok := cfg.CredHelpers[serverAddress]; ok {
		auth, err := getCredentialsFromHelper(helper, serverAddress)
		if err == nil && auth != nil {
			return auth, nil
		}
		// If credential helper fails, continue to try other methods
	}

	// Try global credential store (credsStore)
	if cfg.CredsStore != "" {
		auth, err := getCredentialsFromHelper(cfg.CredsStore, serverAddress)
		if err == nil && auth != nil {
			return auth, nil
		}
		// If credential helper fails, fall through to check auths
	}

	// Try to find matching registry in auths
	for host, auth := range cfg.Auths {
		if matchRegistry(host, registry) {
			// Decode the auth field if present
			if auth.Auth != "" {
				creds, err := base64.StdEncoding.DecodeString(auth.Auth)
				if err != nil {
					return nil, err
				}
				parts := strings.SplitN(string(creds), ":", 2)
				if len(parts) == 2 {
					return &Basic{
						Username: parts[0],
						Password: parts[1],
					}, nil
				}
			}
			if auth.Username != "" && auth.Password != "" {
				return &Basic{
					Username: auth.Username,
					Password: auth.Password,
				}, nil
			}
			if auth.IdentityToken != "" {
				return &Bearer{Token: auth.IdentityToken}, nil
			}
		}
	}

	return nil, nil
}

// getServerAddressForRegistry returns the server address used for credential lookup.
// Docker Hub credentials are stored under "https://index.docker.io/v1/".
func getServerAddressForRegistry(registry string) string {
	// Normalize the registry
	normalized := normalizeRegistry(registry)

	// For Docker Hub, use the standard credential key
	if normalized == "index.docker.io" {
		return "https://index.docker.io/v1/"
	}

	// For other registries, use the normalized registry name
	return normalized
}

// getCredentialsFromHelper retrieves credentials using a Docker credential helper.
// It uses the docker-credential-helpers library to interact with the credential helper.
func getCredentialsFromHelper(helper, serverAddress string) (Authenticator, error) {
	// Use the docker-credential-helpers/client library
	creds, err := credentialHelperGet(helper, serverAddress)
	if err != nil {
		return nil, err
	}

	// nil creds means credentials were not found (not an error)
	if creds == nil {
		return nil, nil
	}

	if creds.Username != "" && creds.Secret != "" {
		return &Basic{Username: creds.Username, Password: creds.Secret}, nil
	}

	return nil, nil
}

// credentialHelperGet uses the docker-credential-helpers/client library
func credentialHelperGet(helper, serverAddress string) (*credentialsResult, error) {
	program := credHelperClient.NewShellProgramFunc("docker-credential-" + helper)
	creds, err := credHelperClient.Get(program, serverAddress)
	if err != nil {
		// Treat "credentials not found" as a miss, not an error
		// This allows fallback to other credential sources
		if credentials.IsErrCredentialsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &credentialsResult{
		Username: creds.Username,
		Secret:   creds.Secret,
	}, nil
}

// credentialsResult holds credentials from a credential helper
type credentialsResult struct {
	Username string
	Secret   string
}

// matchRegistry checks if two registry hostnames match.
func matchRegistry(host, registry string) bool {
	// Normalize hostnames
	host = normalizeRegistry(host)
	registry = normalizeRegistry(registry)
	return host == registry
}

// normalizeRegistry normalizes a registry hostname.
func normalizeRegistry(registry string) string {
	// Remove https:// or http:// prefix
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")
	// Remove trailing slash
	registry = strings.TrimSuffix(registry, "/")
	// Remove /v1 or /v2 suffix (used by Docker Hub in config.json)
	registry = strings.TrimSuffix(registry, "/v1")
	registry = strings.TrimSuffix(registry, "/v2")

	// Handle Docker Hub variations
	switch registry {
	case "docker.io", "registry-1.docker.io", "index.docker.io":
		return "index.docker.io"
	}

	return registry
}

// repositoryResource implements Resource for a repository.
type repositoryResource struct {
	registry string
}

func (r *repositoryResource) RegistryStr() string {
	return r.registry
}

// NewResource creates a Resource from a reference.
func NewResource(ref reference.Reference) Resource {
	return &repositoryResource{
		registry: ref.Context().Registry.RegistryStr(),
	}
}

// FromConfig creates an Authenticator from an AuthConfig.
func FromConfig(cfg AuthConfig) Authenticator {
	if cfg.RegistryToken != "" {
		return &Bearer{Token: cfg.RegistryToken}
	}
	if cfg.IdentityToken != "" {
		return &Bearer{Token: cfg.IdentityToken}
	}
	if cfg.Username != "" || cfg.Password != "" {
		return &Basic{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}
	if cfg.Auth != "" {
		creds, err := base64.StdEncoding.DecodeString(cfg.Auth)
		if err == nil {
			parts := strings.SplitN(string(creds), ":", 2)
			if len(parts) == 2 {
				return &Basic{
					Username: parts[0],
					Password: parts[1],
				}
			}
		}
	}
	return &Anonymous{}
}
