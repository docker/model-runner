package inference

import (
	"testing"
)

func TestValidateRuntimeFlags(t *testing.T) {
	tests := []struct {
		name        string
		flags       []string
		expectError bool
		description string
	}{
		{
			name:        "empty flags",
			flags:       []string{},
			expectError: false,
			description: "Empty array should pass validation",
		},
		{
			name:        "nil flags",
			flags:       nil,
			expectError: false,
			description: "Nil array should pass validation",
		},
		{
			name:        "valid flags without paths",
			flags:       []string{"--verbose", "--debug", "--threads", "4"},
			expectError: false,
			description: "Simple flags without paths should pass",
		},
		{
			name:        "valid single character flags",
			flags:       []string{"-v", "-d", "-t", "4"},
			expectError: false,
			description: "Single character flags should pass",
		},
		{
			name:        "valid flags with numbers and hyphens",
			flags:       []string{"--gpu-memory-utilization", "0.9", "--max-tokens", "1024"},
			expectError: false,
			description: "Flags with hyphens and numeric values should pass",
		},
		{
			name:        "reject absolute path in value",
			flags:       []string{"--log-file", "/var/log/model.log"},
			expectError: true,
			description: "Absolute paths should be rejected",
		},
		{
			name:        "reject absolute path in flag=value format",
			flags:       []string{"--log-file=/var/log/model.log"},
			expectError: true,
			description: "Paths in flag=value format should be rejected",
		},
		{
			name:        "reject relative path with parent directory",
			flags:       []string{"--output", "../file.txt"},
			expectError: true,
			description: "Relative paths with ../ should be rejected",
		},
		{
			name:        "reject relative path with current directory",
			flags:       []string{"--config", "./config.yaml"},
			expectError: true,
			description: "Relative paths with ./ should be rejected",
		},
		{
			name:        "reject Windows-style path with forward slash",
			flags:       []string{"--file", "C:/Users/file.txt"},
			expectError: true,
			description: "Windows-style paths with forward slash should be rejected",
		},
		{
			name:        "reject Windows-style path with backslash",
			flags:       []string{"--file", "C:\\Users\\file.txt"},
			expectError: true,
			description: "Windows-style paths with backslash should be rejected",
		},
		{
			name:        "reject Windows relative path with backslash",
			flags:       []string{"--config", "..\\config.yaml"},
			expectError: true,
			description: "Windows relative paths with backslash should be rejected",
		},
		{
			name:        "reject Windows current directory path",
			flags:       []string{"--output", ".\\output.txt"},
			expectError: true,
			description: "Windows current directory paths should be rejected",
		},
		{
			name:        "reject UNC network path",
			flags:       []string{"--share", "\\\\server\\share\\file.txt"},
			expectError: true,
			description: "UNC network paths should be rejected",
		},
		{
			name:        "reject Windows system path",
			flags:       []string{"--log", "C:\\Windows\\System32\\log.txt"},
			expectError: true,
			description: "Windows system paths should be rejected",
		},
		{
			name:        "reject URL with http",
			flags:       []string{"--endpoint", "http://example.com/api"},
			expectError: true,
			description: "URLs should be rejected (conservative approach)",
		},
		{
			name:        "reject URL with https",
			flags:       []string{"--api-url", "https://api.example.com/v1"},
			expectError: true,
			description: "HTTPS URLs should be rejected (conservative approach)",
		},
		{
			name:        "reject path in middle of flag list",
			flags:       []string{"--verbose", "--log-file", "/tmp/log.txt", "--debug"},
			expectError: true,
			description: "Path anywhere in flag list should be rejected",
		},
		{
			name:        "reject multiple paths",
			flags:       []string{"--input", "/path/to/input", "--output", "/path/to/output"},
			expectError: true,
			description: "Multiple paths should be rejected",
		},
		{
			name:        "reject path traversal attempt",
			flags:       []string{"--file", "../../etc/passwd"},
			expectError: true,
			description: "Path traversal attempts should be rejected",
		},
		{
			name:        "reject root directory",
			flags:       []string{"--root", "/"},
			expectError: true,
			description: "Root directory should be rejected",
		},
		{
			name:        "reject home directory path",
			flags:       []string{"--home", "/home/user/.config"},
			expectError: true,
			description: "Home directory paths should be rejected",
		},
		{
			name:        "valid flag with special characters except slash",
			flags:       []string{"--model-name", "llama-3.2-1b", "--temperature", "0.7"},
			expectError: false,
			description: "Flags with dots, hyphens, and numbers (no slash) should pass",
		},
		{
			name:        "valid flag with underscore",
			flags:       []string{"--max_tokens", "512", "--use_cache"},
			expectError: false,
			description: "Flags with underscores should pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuntimeFlags(tt.flags)

			if tt.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tt.description, err)
				}
			}
		})
	}
}

func TestValidateRuntimeFlags_ErrorMessage(t *testing.T) {
	// Test that error messages are helpful
	flags := []string{"--log-file", "/var/log/test.log"}
	err := ValidateRuntimeFlags(flags)

	if err == nil {
		t.Fatal("Expected error but got none")
	}

	errMsg := err.Error()
	if !contains(errMsg, "/var/log/test.log") {
		t.Errorf("Error message should contain the offending flag value, got: %s", errMsg)
	}
	if !contains(errMsg, "paths are not allowed") {
		t.Errorf("Error message should explain why it failed, got: %s", errMsg)
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(s != "" && indexOf(s, substr) >= 0))
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
