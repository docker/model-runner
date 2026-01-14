package registry

import (
	"os"
	"sync"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/oci/reference"
)

func TestGetDefaultRegistryOptions_NoEnvVars(t *testing.T) {
	// Reset the sync.Once for this test
	resetOnceForTest()

	// Ensure no env vars are set
	os.Unsetenv("DEFAULT_REGISTRY")
	os.Unsetenv("INSECURE_REGISTRY")

	opts := GetDefaultRegistryOptions()

	// WithDefaultOrg is always added
	if len(opts) != 1 {
		t.Errorf("Expected 1 option (WithDefaultOrg), got %d options", len(opts))
	}

	// Verify that the default registry (docker.io) is used when no options are set
	ref, err := reference.ParseReference("myrepo/myimage:tag", opts...)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}

	// When no DEFAULT_REGISTRY is set, the default should be docker.io
	// (distribution/reference normalizes index.docker.io to docker.io)
	expectedRegistry := "docker.io"
	if ref.Context().Registry.Name() != expectedRegistry {
		t.Errorf("Expected default registry to be '%s', got '%s'", expectedRegistry, ref.Context().Registry.Name())
	}

	// Verify it uses HTTPS (secure by default)
	if ref.Context().Registry.Scheme() != "https" {
		t.Errorf("Expected scheme to be 'https', got '%s'", ref.Context().Registry.Scheme())
	}
}

func TestGetDefaultRegistryOptions_OnlyDefaultRegistry(t *testing.T) {
	// Reset the sync.Once for this test
	resetOnceForTest()

	t.Setenv("DEFAULT_REGISTRY", "custom.registry.io")
	os.Unsetenv("INSECURE_REGISTRY")

	opts := GetDefaultRegistryOptions()

	// WithDefaultRegistry + WithDefaultOrg
	if len(opts) != 2 {
		t.Fatalf("Expected 2 options, got %d", len(opts))
	}

	// Verify the option sets the default registry by parsing a reference without explicit registry
	ref, err := reference.ParseReference("myrepo/myimage:tag", opts...)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}

	if ref.Context().Registry.Name() != "custom.registry.io" {
		t.Errorf("Expected registry to be 'custom.registry.io', got '%s'", ref.Context().Registry.Name())
	}

	// Verify it's not insecure (should use https)
	if ref.Context().Registry.Scheme() != "https" {
		t.Errorf("Expected scheme to be 'https', got '%s'", ref.Context().Registry.Scheme())
	}
}

func TestGetDefaultRegistryOptions_OnlyInsecureRegistry(t *testing.T) {
	// Reset the sync.Once for this test
	resetOnceForTest()

	os.Unsetenv("DEFAULT_REGISTRY")
	t.Setenv("INSECURE_REGISTRY", "true")

	opts := GetDefaultRegistryOptions()

	// Insecure + WithDefaultOrg
	if len(opts) != 2 {
		t.Fatalf("Expected 2 options, got %d", len(opts))
	}

	// Verify the option makes the registry insecure by parsing a reference
	ref, err := reference.ParseReference("myregistry.io/myrepo/myimage:tag", opts...)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}

	// Insecure registries should use http
	if ref.Context().Registry.Scheme() != "http" {
		t.Errorf("Expected scheme to be 'http', got '%s'", ref.Context().Registry.Scheme())
	}
}

func TestGetDefaultRegistryOptions_BothEnvVars(t *testing.T) {
	// Reset the sync.Once for this test
	resetOnceForTest()

	t.Setenv("DEFAULT_REGISTRY", "custom.registry.io")
	t.Setenv("INSECURE_REGISTRY", "true")

	opts := GetDefaultRegistryOptions()

	// WithDefaultRegistry + Insecure + WithDefaultOrg
	if len(opts) != 3 {
		t.Fatalf("Expected 3 options, got %d", len(opts))
	}

	// Verify both options are applied
	ref, err := reference.ParseReference("myrepo/myimage:tag", opts...)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}

	// Check custom registry is used
	if ref.Context().Registry.Name() != "custom.registry.io" {
		t.Errorf("Expected registry to be 'custom.registry.io', got '%s'", ref.Context().Registry.Name())
	}

	// Check insecure is applied (http scheme)
	if ref.Context().Registry.Scheme() != "http" {
		t.Errorf("Expected scheme to be 'http', got '%s'", ref.Context().Registry.Scheme())
	}
}

// Helper function to reset the sync.Once for testing
// Note: This is a workaround for testing. In production code, sync.Once ensures
// the initialization only happens once for the lifetime of the program.
func resetOnceForTest() {
	once = sync.Once{}
	defaultRegistryOpts = nil
}

func TestWithTransportNil(t *testing.T) {
	client := NewClient(WithTransport(nil))

	if client.transport == nil {
		t.Error("WithTransport with nil changed transport to nil")
	}

	if client.transport != DefaultTransport {
		t.Error("WithTransport with nil changed the transport from default")
	}
}

func TestWithUserAgentEmpty(t *testing.T) {
	client := NewClient(WithUserAgent(""))

	if client.userAgent == "" {
		t.Error("WithUserAgent with empty string changed user agent to empty")
	}

	if client.userAgent != DefaultUserAgent {
		t.Errorf("WithUserAgent with empty string changed the user agent: got %q, want %q",
			client.userAgent, DefaultUserAgent)
	}
}
