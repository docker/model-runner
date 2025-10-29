package commands

import (
	"os"
	"reflect"
	"testing"
)

func TestExtractFileInclusions(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected []string
	}{
		{
			name:     "simple file",
			prompt:   "Please analyze this @file.txt",
			expected: []string{"file.txt"},
		},
		{
			name:     "quoted file",
			prompt:   "Look at this @\"my file.txt\"",
			expected: []string{"my file.txt"},
		},
		{
			name:     "single quoted file",
			prompt:   "Look at this @'another file.py'",
			expected: []string{"another file.py"},
		},
		{
			name:     "multiple files",
			prompt:   "Check @file1.txt and @file2.go",
			expected: []string{"file1.txt", "file2.go"},
		},
		{
			name:     "relative path",
			prompt:   "Review @./src/main.go",
			expected: []string{"./src/main.go"},
		},
		{
			name:     "absolute path",
			prompt:   "See @/home/user/file.txt",
			expected: []string{"/home/user/file.txt"},
		},
		{
			name:     "mixed quotes and paths",
			prompt:   "Look at @README.md and @\"./src/config.json\"",
			expected: []string{"README.md", "./src/config.json"},
		},
		{
			name:     "no file inclusions",
			prompt:   "Just a regular prompt",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFileInclusions(tt.prompt)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ExtractFileInclusions() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProcessFileInclusions(t *testing.T) {
	// Create a temporary file for testing in the system's temp directory
	tempDir := t.TempDir()
	tempFile := tempDir + "/test_process_file_inclusions_temp_file.txt"
	tempContent := "This is test file content"

	// Write test content to a temporary file in the temp directory
	err := os.WriteFile(tempFile, []byte(tempContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Ensure cleanup happens even if test fails
	defer func() {
		os.Remove(tempFile)
	}()

	// Test with the temporary file
	prompt := "Analyze this @" + tempFile + " file"
	expected := "Analyze this " + tempContent + " file"

	result, err := ProcessFileInclusions(prompt)
	if err != nil {
		t.Errorf("ProcessFileInclusions() error = %v", err)
		return
	}

	if result != expected {
		t.Errorf("ProcessFileInclusions() = %v, want %v", result, expected)
	}

	// Test with a file that doesn't exist (should be skipped)
	prompt2 := "Analyze this @nonexistent.txt file"
	expected2 := "Analyze this @nonexistent.txt file" // Should remain unchanged since file doesn't exist

	result2, err := ProcessFileInclusions(prompt2)
	if err != nil {
		t.Errorf("ProcessFileInclusions() error = %v", err)
		return
	}

	if result2 != expected2 {
		t.Errorf("ProcessFileInclusions() with non-existent file = %v, want %v", result2, expected2)
	}
}
