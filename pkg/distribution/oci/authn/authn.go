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

	// Try to find matching registry
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

	// Handle Docker Hub variations
	switch registry {
	case "docker.io", "registry-1.docker.io":
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
