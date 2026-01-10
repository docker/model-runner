package distribution

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/tarball"
	"github.com/sirupsen/logrus"
)

func TestNormalizeModelName(t *testing.T) {
	// Create a client with a temporary store for testing
	client, cleanup := createTestClient(t)
	defer cleanup()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{
			name:     "short name only",
			input:    "gemma3",
			expected: "ai/gemma3:latest",
		},
		{
			name:     "short name with tag",
			input:    "gemma3:v1",
			expected: "ai/gemma3:v1",
		},
		{
			name:     "org and name without tag",
			input:    "myorg/model",
			expected: "myorg/model:latest",
		},
		{
			name:     "org and name with tag",
			input:    "myorg/model:v2",
			expected: "myorg/model:v2",
		},
		{
			name:     "fully qualified reference",
			input:    "ai/gemma3:latest",
			expected: "ai/gemma3:latest",
		},

		// Registry cases
		{
			name:     "registry without tag",
			input:    "registry.example.com/model",
			expected: "registry.example.com/model:latest",
		},
		{
			name:     "registry with tag",
			input:    "registry.example.com/model:v1",
			expected: "registry.example.com/model:v1",
		},
		{
			name:     "registry with org and tag",
			input:    "registry.example.com/myorg/model:v1",
			expected: "registry.example.com/myorg/model:v1",
		},

		// ID cases - without store lookup (IDs not in store)
		{
			name:     "short ID (12 hex chars) not in store",
			input:    "1234567890ab",
			expected: "1234567890ab", // Returns as-is since not found
		},
		{
			name:     "long ID (64 hex chars) not in store",
			input:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:     "sha256 digest not in store",
			input:    "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "name with leading/trailing whitespace",
			input:    "  gemma3  ",
			expected: "ai/gemma3:latest",
		},
		{
			name:     "name with trailing colon (no tag)",
			input:    "model:",
			expected: "ai/model:latest",
		},
		{
			name:     "org/name with trailing colon",
			input:    "myorg/model:",
			expected: "myorg/model:latest",
		},
		{
			name:     "name that looks like hex but wrong length",
			input:    "abc123",
			expected: "ai/abc123:latest",
		},
		{
			name:     "name with non-hex characters",
			input:    "model-xyz",
			expected: "ai/model-xyz:latest",
		},
		{
			name:     "name with uppercase (not huggingface)",
			input:    "MyModel",
			expected: "ai/mymodel:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.normalizeModelName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLooksLikeID(t *testing.T) {
	// Create a client for testing
	client, cleanup := createTestClient(t)
	defer cleanup()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "short ID valid",
			input:    "1234567890ab",
			expected: true,
		},
		{
			name:     "long ID valid",
			input:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: true,
		},
		{
			name:     "too short",
			input:    "12345",
			expected: false,
		},
		{
			name:     "too long",
			input:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
			expected: false,
		},
		{
			name:     "non-hex characters in short",
			input:    "12345678xyz9",
			expected: false,
		},
		{
			name:     "non-hex characters in long",
			input:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcXYZ",
			expected: false,
		},
		{
			name:     "uppercase hex",
			input:    "1234567890AB",
			expected: false,
		},
		{
			name:     "empty",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.looksLikeID(tt.input)
			if result != tt.expected {
				t.Errorf("looksLikeID(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLooksLikeDigest(t *testing.T) {
	// Create a client for testing
	client, cleanup := createTestClient(t)
	defer cleanup()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid digest",
			input:    "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: true,
		},
		{
			name:     "missing prefix",
			input:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: false,
		},
		{
			name:     "wrong prefix",
			input:    "sha512:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: false,
		},
		{
			name:     "invalid hash after prefix",
			input:    "sha256:invalid",
			expected: false,
		},
		{
			name:     "short hash after prefix",
			input:    "sha256:1234567890ab",
			expected: false,
		},
		{
			name:     "uppercase hex in hash",
			input:    "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
			expected: false,
		},
		{
			name:     "empty",
			input:    "",
			expected: false,
		},
		{
			name:     "prefix only",
			input:    "sha256:",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.looksLikeDigest(tt.input)
			if result != tt.expected {
				t.Errorf("looksLikeDigest(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeModelNameWithIDResolution(t *testing.T) {
	// Create a client with a temporary store
	client, cleanup := createTestClient(t)
	defer cleanup()

	// Load a test model to get a real ID
	testGGUFFile := filepath.Join("..", "assets", "dummy.gguf")
	modelID := loadTestModel(t, client, testGGUFFile)

	// Extract the short ID (12 hex chars after "sha256:")
	if !strings.HasPrefix(modelID, "sha256:") {
		t.Fatalf("Expected model ID to start with 'sha256:', got: %s", modelID)
	}
	shortID := modelID[7:19] // Extract 12 chars after "sha256:"
	fullHex := strings.TrimPrefix(modelID, "sha256:")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short ID resolves to full ID",
			input:    shortID,
			expected: modelID,
		},
		{
			name:     "full hex (without sha256:) resolves to full ID",
			input:    fullHex,
			expected: modelID,
		},
		{
			name:     "full digest (with sha256:) returns as-is",
			input:    modelID,
			expected: modelID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.normalizeModelName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to create a test client with temp store
func createTestClient(t *testing.T) (*Client, func()) {
	t.Helper()

	// Create temp directory for store
	tempDir := t.TempDir()

	// Create client with minimal config
	client, err := NewClient(
		WithStoreRootPath(tempDir),
		WithLogger(logrus.NewEntry(logrus.StandardLogger())),
	)
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	cleanup := func() {
		if err := client.ResetStore(); err != nil {
			t.Logf("Warning: failed to reset store: %v", err)
		}
	}

	return client, cleanup
}

func TestIsHuggingFaceReference(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"huggingface.co prefix", "huggingface.co/org/model:latest", true},
		{"huggingface.co without tag", "huggingface.co/org/model", true},
		{"not huggingface", "registry.example.com/model:latest", false},
		{"docker hub", "ai/gemma3:latest", false},
		{"hf.co prefix (short form)", "hf.co/org/model", true}, // Short form is also recognized
		{"hf.co with quantization", "hf.co/bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHuggingFaceReference(tt.input)
			if result != tt.expected {
				t.Errorf("isHuggingFaceReference(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseHFReference(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedRepo string
		expectedRev  string
		expectedTag  string
	}{
		{
			name:         "basic with latest tag",
			input:        "huggingface.co/org/model:latest",
			expectedRepo: "org/model",
			expectedRev:  "main", // revision is always main
			expectedTag:  "latest",
		},
		{
			name:         "with quantization tag",
			input:        "huggingface.co/org/model:Q4_K_M",
			expectedRepo: "org/model",
			expectedRev:  "main",
			expectedTag:  "Q4_K_M",
		},
		{
			name:         "without tag",
			input:        "huggingface.co/org/model",
			expectedRepo: "org/model",
			expectedRev:  "main",
			expectedTag:  "latest",
		},
		{
			name:         "with commit hash as tag",
			input:        "huggingface.co/HuggingFaceTB/SmolLM2-135M-Instruct:abc123",
			expectedRepo: "HuggingFaceTB/SmolLM2-135M-Instruct",
			expectedRev:  "main",
			expectedTag:  "abc123",
		},
		{
			name:         "single name (no org)",
			input:        "huggingface.co/model:latest",
			expectedRepo: "model",
			expectedRev:  "main",
			expectedTag:  "latest",
		},
		{
			name:         "hf.co prefix with quantization",
			input:        "hf.co/bartowski/Llama-3.2-1B-Instruct-GGUF:Q8_0",
			expectedRepo: "bartowski/Llama-3.2-1B-Instruct-GGUF",
			expectedRev:  "main",
			expectedTag:  "Q8_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, rev, tag := parseHFReference(tt.input)
			if repo != tt.expectedRepo {
				t.Errorf("parseHFReference(%q) repo = %q, want %q", tt.input, repo, tt.expectedRepo)
			}
			if rev != tt.expectedRev {
				t.Errorf("parseHFReference(%q) rev = %q, want %q", tt.input, rev, tt.expectedRev)
			}
			if tag != tt.expectedTag {
				t.Errorf("parseHFReference(%q) tag = %q, want %q", tt.input, tag, tt.expectedTag)
			}
		})
	}
}

// Helper function to load a test model and return its ID
func loadTestModel(t *testing.T, client *Client, ggufPath string) string {
	t.Helper()

	// Load model using LoadModel
	pr, pw := io.Pipe()
	target, err := tarball.NewTarget(pw)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	done := make(chan error)
	var id string
	go func() {
		var err error
		id, err = client.LoadModel(pr, nil)
		done <- err
	}()

	bldr, err := builder.FromPath(ggufPath)
	if err != nil {
		t.Fatalf("Failed to create builder from GGUF: %v", err)
	}

	ctx := context.Background()
	if err := bldr.Build(ctx, target, nil); err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	if id == "" {
		t.Fatal("Model ID is empty")
	}

	return id
}
