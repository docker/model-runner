//go:build !windows

package readline

import (
	"os"
	"path/filepath"
	"testing"
)

func createMockEditor(t *testing.T, scriptBody string) string {
	t.Helper()
	editorScript := filepath.Join(t.TempDir(), "mock-editor.sh")
	if err := os.WriteFile(editorScript, []byte("#!/bin/sh\n"+scriptBody+"\n"), 0o755); err != nil {
		t.Fatalf("failed to create mock editor: %v", err)
	}
	t.Setenv("EDITOR", editorScript)
	return editorScript
}

func TestRunEditor(t *testing.T) {
	tests := []struct {
		name             string
		mockEditorScript string
		input            string
		expected         string
	}{
		{
			name:             "modifies content",
			mockEditorScript: `printf " edited" >> "$1"`,
			input:            "hello docker model prompt",
			expected:         "hello docker model prompt edited",
		},
		{
			name:             "empty content",
			mockEditorScript: `printf "new content" > "$1"`,
			input:            "",
			expected:         "new content",
		},
		{
			name:             "strips trailing newline",
			mockEditorScript: `printf "edited\n" > "$1"`,
			input:            "",
			expected:         "edited",
		},
		{
			name:             "strips trailing carriage return and newline",
			mockEditorScript: `printf "edited\r\n" > "$1"`,
			input:            "",
			expected:         "edited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createMockEditor(t, tt.mockEditorScript)

			result, err := runEditor(tt.input)
			if err != nil {
				t.Fatalf("runEditor failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRunEditorReturnsOriginalContentOnFailure(t *testing.T) {
	t.Setenv("EDITOR", "non_exists_editor")

	content := "docker model prompt hello"
	result, err := runEditor(content)
	if err == nil {
		t.Fatal("expected error from nonexistent editor")
	}

	if result != content {
		t.Errorf("expected original content on failure, got %q", result)
	}
}

func TestRunEditorWithEditorArgs(t *testing.T) {
	editorScript := createMockEditor(t, `printf "edited with args" > "$2"`)
	t.Setenv("EDITOR", editorScript+" --wait")

	result, err := runEditor("original")
	if err != nil {
		t.Fatalf("runEditor failed: %v", err)
	}

	if result != "edited with args" {
		t.Errorf("expected %q, got %q", "edited with args", result)
	}
}
