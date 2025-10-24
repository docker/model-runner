package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeCommandFlags(t *testing.T) {
	cmd := newCodeCmd()

	// Test that the command has the expected flags
	flags := []string{"backend", "aider-image"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q to be defined", flag)
		}
	}

	// Test default aider-image value
	aiderImage, _ := cmd.Flags().GetString("aider-image")
	if aiderImage != "paulgauthier/aider" {
		t.Errorf("expected default aider-image to be 'paulgauthier/aider', got %q", aiderImage)
	}
}

func TestCodeCommandArgsValidation(t *testing.T) {
	cmd := newCodeCmd()

	// Test that command requires at least one argument
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("expected error when no arguments provided")
	}

	// Test that command accepts one argument
	if err := cmd.Args(cmd, []string{"model"}); err != nil {
		t.Errorf("expected no error with one argument, got: %v", err)
	}

	// Test that command accepts multiple arguments
	if err := cmd.Args(cmd, []string{"model", "prompt"}); err != nil {
		t.Errorf("expected no error with multiple arguments, got: %v", err)
	}
}

func TestGetPromptFromEditor(t *testing.T) {
	// Save original environment
	origEditor := os.Getenv("EDITOR")
	origVisual := os.Getenv("VISUAL")
	defer func() {
		os.Setenv("EDITOR", origEditor)
		os.Setenv("VISUAL", origVisual)
	}()

	// Test with a mock editor that writes to the temp file
	tmpDir := t.TempDir()
	mockEditor := filepath.Join(tmpDir, "mock-editor.sh")
	mockScript := `#!/bin/bash
echo "Test prompt content" >> "$1"
`
	if err := os.WriteFile(mockEditor, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to create mock editor: %v", err)
	}

	os.Setenv("EDITOR", mockEditor)
	os.Unsetenv("VISUAL")

	prompt, err := getPromptFromEditor()
	if err != nil {
		t.Fatalf("getPromptFromEditor() returned error: %v", err)
	}

	if !strings.Contains(prompt, "Test prompt content") {
		t.Errorf("expected prompt to contain 'Test prompt content', got: %q", prompt)
	}
}

func TestGetPromptFromEditorFiltersComments(t *testing.T) {
	// Save original environment
	origEditor := os.Getenv("EDITOR")
	origVisual := os.Getenv("VISUAL")
	defer func() {
		os.Setenv("EDITOR", origEditor)
		os.Setenv("VISUAL", origVisual)
	}()

	// Test with a mock editor that writes comments and content
	tmpDir := t.TempDir()
	mockEditor := filepath.Join(tmpDir, "mock-editor.sh")
	mockScript := `#!/bin/bash
cat >> "$1" << 'EOF'
# This is a comment
This is content
# Another comment
More content
EOF
`
	if err := os.WriteFile(mockEditor, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to create mock editor: %v", err)
	}

	os.Setenv("EDITOR", mockEditor)
	os.Unsetenv("VISUAL")

	prompt, err := getPromptFromEditor()
	if err != nil {
		t.Fatalf("getPromptFromEditor() returned error: %v", err)
	}

	if strings.Contains(prompt, "# This is a comment") {
		t.Error("expected comments to be filtered out")
	}

	if !strings.Contains(prompt, "This is content") {
		t.Error("expected content line to be present")
	}

	if !strings.Contains(prompt, "More content") {
		t.Error("expected second content line to be present")
	}
}

func TestRemoveElement(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		element  string
		expected []string
	}{
		{
			name:     "remove single occurrence",
			slice:    []string{"a", "b", "c"},
			element:  "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove multiple occurrences",
			slice:    []string{"a", "b", "c", "b"},
			element:  "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "element not present",
			slice:    []string{"a", "b", "c"},
			element:  "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			slice:    []string{},
			element:  "a",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeElement(tt.slice, tt.element)
			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("expected %v, got %v", tt.expected, result)
					return
				}
			}
		})
	}
}

func TestGetModelRunnerURL(t *testing.T) {
	// Save original environment
	origHost := os.Getenv("MODEL_RUNNER_HOST")
	defer func() {
		if origHost != "" {
			os.Setenv("MODEL_RUNNER_HOST", origHost)
		} else {
			os.Unsetenv("MODEL_RUNNER_HOST")
		}
	}()

	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "no environment variable",
			envValue: "",
			expected: "http://localhost:12434/engines/v1/",
		},
		{
			name:     "custom host with trailing slash",
			envValue: "http://example.com/",
			expected: "http://example.com/engines/v1/",
		},
		{
			name:     "custom host without trailing slash",
			envValue: "http://example.com",
			expected: "http://example.com/engines/v1/",
		},
		{
			name:     "custom host with engines/v1/ path",
			envValue: "http://example.com/engines/v1/",
			expected: "http://example.com/engines/v1/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("MODEL_RUNNER_HOST", tt.envValue)
			} else {
				os.Unsetenv("MODEL_RUNNER_HOST")
			}

			result := getModelRunnerURL()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
