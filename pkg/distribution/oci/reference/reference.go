// Package reference provides image reference parsing using the distribution/reference library.
// This replaces go-containerregistry's name package.
package reference

import (
	"fmt"
	"os"
	"strings"

	"github.com/distribution/reference"
)

const (
	// DefaultRegistry is the default registry (Docker Hub).
	DefaultRegistry = "index.docker.io"
	// DefaultTag is the default tag when none is specified.
	DefaultTag = "latest"
)

// Reference represents an image reference.
type Reference interface {
	// Name returns the full name of the reference (registry/repo).
	Name() string
	// String returns the full reference string.
	String() string
	// Context returns the repository context.
	Context() Repository
	// Identifier returns the tag or digest identifier.
	Identifier() string
	// Scope returns the scope for registry authentication.
	Scope(action string) string
}

// Repository represents a repository context.
type Repository struct {
	Registry   Registry
	Repository string
}

// Name returns the full repository name including registry.
func (r Repository) Name() string {
	if r.Registry.Name() == DefaultRegistry {
		return r.Repository
	}
	return r.Registry.Name() + "/" + r.Repository
}

// RepositoryStr returns just the repository part.
func (r Repository) RepositoryStr() string {
	return r.Repository
}

// Registry represents a registry.
type Registry struct {
	registry string
	insecure bool
}

// Name returns the registry name.
func (r Registry) Name() string {
	return r.registry
}

// RegistryStr returns the registry string.
func (r Registry) RegistryStr() string {
	return r.registry
}

// Scheme returns the URL scheme (http or https).
func (r Registry) Scheme() string {
	if r.insecure || isInsecureHost(r.registry) {
		return "http"
	}
	return "https"
}

// isInsecureHost returns true if the host should use HTTP by default.
// This includes localhost and .local hostnames.
func isInsecureHost(host string) bool {
	// Remove port if present
	hostWithoutPort := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostWithoutPort = host[:idx]
	}

	// Check for localhost
	if hostWithoutPort == "localhost" {
		return true
	}

	// Check for .local suffix (mDNS/Bonjour)
	if strings.HasSuffix(hostWithoutPort, ".local") {
		return true
	}

	return false
}

// Tag represents a tagged image reference.
type Tag struct {
	ref        reference.Named
	registry   Registry
	repository string
	tag        string
}

// Name returns the full reference name.
func (t *Tag) Name() string {
	return t.Context().Name()
}

// String returns the full reference string including tag.
func (t *Tag) String() string {
	return fmt.Sprintf("%s/%s:%s", t.registry.Name(), t.repository, t.tag)
}

// Context returns the repository context.
func (t *Tag) Context() Repository {
	return Repository{
		Registry:   t.registry,
		Repository: t.repository,
	}
}

// Identifier returns the tag.
func (t *Tag) Identifier() string {
	return t.tag
}

// TagStr returns just the tag string.
func (t *Tag) TagStr() string {
	return t.tag
}

// Scope returns the scope for registry authentication.
func (t *Tag) Scope(action string) string {
	return fmt.Sprintf("repository:%s:%s", t.repository, action)
}

// Digest represents a digest-referenced image.
type Digest struct {
	ref        reference.Named
	registry   Registry
	repository string
	digest     string
}

// Name returns the full reference name.
func (d *Digest) Name() string {
	return d.Context().Name()
}

// String returns the full reference string including digest.
func (d *Digest) String() string {
	return fmt.Sprintf("%s/%s@%s", d.registry.Name(), d.repository, d.digest)
}

// Context returns the repository context.
func (d *Digest) Context() Repository {
	return Repository{
		Registry:   d.registry,
		Repository: d.repository,
	}
}

// Identifier returns the digest.
func (d *Digest) Identifier() string {
	return d.digest
}

// DigestStr returns just the digest string.
func (d *Digest) DigestStr() string {
	return d.digest
}

// Scope returns the scope for registry authentication.
func (d *Digest) Scope(action string) string {
	return fmt.Sprintf("repository:%s:%s", d.repository, action)
}

// Option is a functional option for reference parsing.
type Option func(*options)

type options struct {
	defaultRegistry string
	defaultOrg      string
	insecure        bool
}

// WithDefaultRegistry sets a custom default registry.
func WithDefaultRegistry(registry string) Option {
	return func(o *options) {
		o.defaultRegistry = registry
	}
}

// WithDefaultOrg sets a custom default organization.
// This is used when a reference doesn't include an organization (e.g., "model:tag").
func WithDefaultOrg(org string) Option {
	return func(o *options) {
		o.defaultOrg = org
	}
}

// Insecure allows insecure (HTTP) connections.
var Insecure Option = func(o *options) {
	o.insecure = true
}

// ParseReference parses a string into a Reference.
func ParseReference(s string, opts ...Option) (Reference, error) {
	o := &options{
		defaultRegistry: DefaultRegistry,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Detect if the original reference has an explicit registry or org
	hasExplicitRegistry := false
	hasExplicitOrg := false

	// Find the first "/" to separate potential registry from the rest
	firstSlash := strings.Index(s, "/")
	if firstSlash > 0 {
		firstPart := s[:firstSlash]
		// A registry typically contains a dot or colon (e.g., example.com or localhost:5000)
		hasExplicitRegistry = strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":")

		if hasExplicitRegistry {
			// If there's an explicit registry, check for a second "/" which indicates an org
			rest := s[firstSlash+1:]
			hasExplicitOrg = strings.Contains(rest, "/")
		} else {
			// If the first part is not a registry, it's an org
			hasExplicitOrg = true
		}
	}

	// Handle references without a registry by adding the default
	ref, err := reference.ParseNormalizedNamed(s)
	if err != nil {
		return nil, fmt.Errorf("invalid reference %q: %w", s, err)
	}

	// Get registry and repository
	domain := reference.Domain(ref)
	// If no explicit registry was specified and we have a custom default, use it
	if !hasExplicitRegistry && o.defaultRegistry != DefaultRegistry {
		domain = o.defaultRegistry
	}
	path := reference.Path(ref)

	// If no explicit org was specified and we have a custom default org, use it
	// The distribution/reference library adds "library/" for official images
	if !hasExplicitOrg && o.defaultOrg != "" && strings.HasPrefix(path, "library/") {
		path = o.defaultOrg + "/" + strings.TrimPrefix(path, "library/")
	}

	registry := Registry{
		registry: domain,
		insecure: o.insecure,
	}

	// Check if it's a tagged reference
	if tagged, ok := ref.(reference.Tagged); ok {
		return &Tag{
			ref:        ref,
			registry:   registry,
			repository: path,
			tag:        tagged.Tag(),
		}, nil
	}

	// Check if it's a digested reference
	if digested, ok := ref.(reference.Digested); ok {
		return &Digest{
			ref:        ref,
			registry:   registry,
			repository: path,
			digest:     digested.Digest().String(),
		}, nil
	}

	// Default to latest tag
	ref = reference.TagNameOnly(ref)
	return &Tag{
		ref:        ref,
		registry:   registry,
		repository: path,
		tag:        DefaultTag,
	}, nil
}

// NewTag creates a new tag reference.
func NewTag(s string, opts ...Option) (*Tag, error) {
	ref, err := ParseReference(s, opts...)
	if err != nil {
		return nil, err
	}
	if tag, ok := ref.(*Tag); ok {
		return tag, nil
	}
	return nil, fmt.Errorf("reference %q is not a tag", s)
}

// NewDigest creates a new digest reference.
func NewDigest(s string, opts ...Option) (*Digest, error) {
	ref, err := ParseReference(s, opts...)
	if err != nil {
		return nil, err
	}
	if digest, ok := ref.(*Digest); ok {
		return digest, nil
	}
	return nil, fmt.Errorf("reference %q is not a digest", s)
}

// DefaultOrg is the default organization when none is specified.
const DefaultOrg = "ai"

// GetDefaultRegistryOptions returns options based on environment variables.
func GetDefaultRegistryOptions() []Option {
	var opts []Option
	if defaultReg := os.Getenv("DEFAULT_REGISTRY"); defaultReg != "" {
		opts = append(opts, WithDefaultRegistry(defaultReg))
	}
	if os.Getenv("INSECURE_REGISTRY") == "true" {
		opts = append(opts, Insecure)
	}
	// Always use the default org for consistency with model-runner's normalization
	opts = append(opts, WithDefaultOrg(DefaultOrg))
	return opts
}

// Domain returns the domain part of a reference.
func Domain(ref Reference) string {
	return ref.Context().Registry.Name()
}

// Path returns the path part of a reference (repository without registry).
func Path(ref Reference) string {
	return ref.Context().Repository
}

// IsDockerHub checks if the reference points to Docker Hub.
func IsDockerHub(ref Reference) bool {
	domain := ref.Context().Registry.Name()
	return domain == "docker.io" || domain == "index.docker.io" || domain == "registry-1.docker.io"
}

// Normalize normalizes a reference string to include registry and tag if missing.
func Normalize(s string) string {
	ref, err := ParseReference(s)
	if err != nil {
		return s
	}
	return ref.String()
}

// SplitReference splits a reference string into registry, repository, and tag/digest.
func SplitReference(s string) (registry, repository, identifier string) {
	ref, err := ParseReference(s)
	if err != nil {
		return "", "", ""
	}
	return ref.Context().Registry.Name(), ref.Context().Repository, ref.Identifier()
}

// FixDockerHubLibrary adds "library/" prefix for official Docker Hub images.
func FixDockerHubLibrary(ref Reference) string {
	if !IsDockerHub(ref) {
		return ref.String()
	}
	repo := ref.Context().Repository
	if !strings.Contains(repo, "/") {
		// Official image, add library prefix
		repo = "library/" + repo
	}
	return fmt.Sprintf("%s/%s:%s", ref.Context().Registry.Name(), repo, ref.Identifier())
}
