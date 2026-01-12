package authn

import "testing"

func TestNormalizeRegistry(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Docker Hub variations
		{
			name:     "docker.io",
			input:    "docker.io",
			expected: "index.docker.io",
		},
		{
			name:     "registry-1.docker.io",
			input:    "registry-1.docker.io",
			expected: "index.docker.io",
		},
		{
			name:     "index.docker.io",
			input:    "index.docker.io",
			expected: "index.docker.io",
		},
		// Docker Hub with /v1 suffix (as stored in ~/.docker/config.json)
		{
			name:     "https://index.docker.io/v1/",
			input:    "https://index.docker.io/v1/",
			expected: "index.docker.io",
		},
		{
			name:     "index.docker.io/v1",
			input:    "index.docker.io/v1",
			expected: "index.docker.io",
		},
		{
			name:     "https://index.docker.io/v1",
			input:    "https://index.docker.io/v1",
			expected: "index.docker.io",
		},
		// Docker Hub with /v2 suffix
		{
			name:     "index.docker.io/v2",
			input:    "index.docker.io/v2",
			expected: "index.docker.io",
		},
		{
			name:     "https://index.docker.io/v2/",
			input:    "https://index.docker.io/v2/",
			expected: "index.docker.io",
		},
		// With https:// prefix
		{
			name:     "https://docker.io",
			input:    "https://docker.io",
			expected: "index.docker.io",
		},
		{
			name:     "https://index.docker.io",
			input:    "https://index.docker.io",
			expected: "index.docker.io",
		},
		// With http:// prefix
		{
			name:     "http://docker.io",
			input:    "http://docker.io",
			expected: "index.docker.io",
		},
		// With trailing slash
		{
			name:     "docker.io/",
			input:    "docker.io/",
			expected: "index.docker.io",
		},
		{
			name:     "https://docker.io/",
			input:    "https://docker.io/",
			expected: "index.docker.io",
		},
		// Other registries (should remain unchanged)
		{
			name:     "gcr.io",
			input:    "gcr.io",
			expected: "gcr.io",
		},
		{
			name:     "ghcr.io",
			input:    "ghcr.io",
			expected: "ghcr.io",
		},
		{
			name:     "quay.io",
			input:    "quay.io",
			expected: "quay.io",
		},
		{
			name:     "custom registry",
			input:    "registry.example.com",
			expected: "registry.example.com",
		},
		{
			name:     "custom registry with https",
			input:    "https://registry.example.com",
			expected: "registry.example.com",
		},
		{
			name:     "custom registry with port",
			input:    "localhost:5000",
			expected: "localhost:5000",
		},
		{
			name:     "ECR registry",
			input:    "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expected: "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRegistry(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeRegistry(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchRegistry(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		registry string
		expected bool
	}{
		// Docker Hub matching - the key use case for this fix
		{
			name:     "config key matches docker.io",
			host:     "https://index.docker.io/v1/",
			registry: "docker.io",
			expected: true,
		},
		{
			name:     "config key matches index.docker.io",
			host:     "https://index.docker.io/v1/",
			registry: "index.docker.io",
			expected: true,
		},
		{
			name:     "docker.io matches docker.io",
			host:     "docker.io",
			registry: "docker.io",
			expected: true,
		},
		{
			name:     "docker.io matches index.docker.io",
			host:     "docker.io",
			registry: "index.docker.io",
			expected: true,
		},
		{
			name:     "registry-1.docker.io matches docker.io",
			host:     "registry-1.docker.io",
			registry: "docker.io",
			expected: true,
		},
		// Non-matching registries
		{
			name:     "different registries",
			host:     "gcr.io",
			registry: "docker.io",
			expected: false,
		},
		{
			name:     "ghcr.io vs docker.io",
			host:     "ghcr.io",
			registry: "docker.io",
			expected: false,
		},
		// Same registries
		{
			name:     "gcr.io matches gcr.io",
			host:     "gcr.io",
			registry: "gcr.io",
			expected: true,
		},
		{
			name:     "https://gcr.io matches gcr.io",
			host:     "https://gcr.io",
			registry: "gcr.io",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchRegistry(tt.host, tt.registry)
			if result != tt.expected {
				t.Errorf("matchRegistry(%q, %q) = %v, want %v", tt.host, tt.registry, result, tt.expected)
			}
		})
	}
}
