package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathWithinDirectory(t *testing.T) {
	// Create a temporary directory for testing
	baseDir := t.TempDir()

	tests := []struct {
		name        string
		targetPath  string
		expectError bool
		description string
	}{
		// Valid paths - should pass
		{
			name:        "simple filename",
			targetPath:  "model.safetensors",
			expectError: false,
			description: "Simple filename should be valid",
		},
		{
			name:        "nested directory",
			targetPath:  "text_encoder/model.safetensors",
			expectError: false,
			description: "Nested path should be valid",
		},
		{
			name:        "deeply nested",
			targetPath:  "a/b/c/d/model.safetensors",
			expectError: false,
			description: "Deeply nested path should be valid",
		},

		// Directory traversal attacks - should fail
		{
			name:        "parent directory escape",
			targetPath:  "../etc/passwd",
			expectError: true,
			description: "Parent directory escape should be blocked",
		},
		{
			name:        "multiple parent escape",
			targetPath:  "../../../etc/passwd",
			expectError: true,
			description: "Multiple parent directory escape should be blocked",
		},
		{
			name:        "mixed path with escape",
			targetPath:  "text_encoder/../../../etc/passwd",
			expectError: true,
			description: "Path that starts valid but escapes should be blocked",
		},
		{
			name:        "absolute path unix",
			targetPath:  "/etc/passwd",
			expectError: true,
			description: "Absolute Unix path should be blocked",
		},
		// Note: Windows absolute path test is platform-specific.
		// On Unix, "C:\..." is treated as a relative path (it doesn't start with /),
		// so it would create a file/directory with that name, which is allowed.
		// On Windows, filepath.IsAbs() correctly identifies "C:\" as absolute.

		// Edge cases
		{
			name:        "empty path",
			targetPath:  "",
			expectError: true,
			description: "Empty path should be blocked (filepath.IsLocal returns false for empty)",
		},
		{
			name:        "dot only",
			targetPath:  ".",
			expectError: true,
			description: "Dot path should be blocked",
		},
		{
			name:        "double dot only",
			targetPath:  "..",
			expectError: true,
			description: "Double dot path should be blocked",
		},
		{
			name:        "path with null byte",
			targetPath:  "model\x00.safetensors",
			expectError: true,
			description: "Path with null byte should be blocked (invalid in most filesystems)",
		},

		// Tricky paths that might bypass naive checks
		{
			name:        ".. in middle",
			targetPath:  "foo/../bar/model.safetensors",
			expectError: false,
			description: "Path with .. that stays within directory should be valid",
		},
		{
			name:        "trailing slash",
			targetPath:  "text_encoder/",
			expectError: false,
			description: "Directory path with trailing slash should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathWithinDirectory(baseDir, tt.targetPath)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for path %q (%s), but got nil", tt.targetPath, tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for path %q (%s), but got: %v", tt.targetPath, tt.description, err)
			}
		})
	}
}

func TestValidatePathWithinDirectory_RealFilesystem(t *testing.T) {
	// Create a temporary directory structure
	baseDir := t.TempDir()

	// Create a sibling directory that attacker might try to access
	siblingDir := filepath.Join(filepath.Dir(baseDir), "sibling-secret")
	if err := os.MkdirAll(siblingDir, 0755); err != nil {
		t.Fatalf("Failed to create sibling dir: %v", err)
	}
	defer os.RemoveAll(siblingDir)

	// Create a secret file in the sibling directory
	secretFile := filepath.Join(siblingDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret data"), 0644); err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	// Try to escape to the sibling directory
	escapePath := "../sibling-secret/secret.txt"
	err := validatePathWithinDirectory(baseDir, escapePath)
	if err == nil {
		t.Errorf("Expected error when attempting to escape to sibling directory, but validation passed")
	}
}
