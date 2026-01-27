package inference

import (
	"strings"
	"testing"
)

func TestValidateRuntimeFlags(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		flags       []string
		expectError bool
		description string
	}{
		// Tests for llama.cpp backend with allowlist
		{
			name:        "llama.cpp: empty flags",
			backend:     "llama.cpp",
			flags:       []string{},
			expectError: false,
			description: "Empty array should pass validation",
		},
		{
			name:        "llama.cpp: nil flags",
			backend:     "llama.cpp",
			flags:       nil,
			expectError: false,
			description: "Nil array should pass validation",
		},
		{
			name:        "llama.cpp: valid allowed flags",
			backend:     "llama.cpp",
			flags:       []string{"--verbose", "--threads", "4"},
			expectError: false,
			description: "Allowed flags should pass",
		},
		{
			name:        "llama.cpp: valid single character flags",
			backend:     "llama.cpp",
			flags:       []string{"-v", "-t", "4"},
			expectError: false,
			description: "Single character allowed flags should pass",
		},
		{
			name:        "llama.cpp: flag with equals format",
			backend:     "llama.cpp",
			flags:       []string{"--threads=4", "--ctx-size=2048"},
			expectError: false,
			description: "Flags with = format should pass",
		},
		{
			name:        "llama.cpp: reject non-allowed flag",
			backend:     "llama.cpp",
			flags:       []string{"--log-file", "test.log"},
			expectError: true,
			description: "Non-allowed flags should be rejected",
		},
		{
			name:        "llama.cpp: reject path in allowed flag value",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "/etc/passwd"},
			expectError: true,
			description: "Paths in values should be rejected",
		},
		{
			name:        "llama.cpp: reject path in flag=value format",
			backend:     "llama.cpp",
			flags:       []string{"--threads=/var/log/test"},
			expectError: true,
			description: "Paths in flag=value format should be rejected",
		},
		{
			name:        "llama.cpp: multiple allowed flags",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "4", "--ctx-size", "2048", "--verbose", "--flash-attn"},
			expectError: false,
			description: "Multiple allowed flags should pass",
		},
		{
			name:        "llama.cpp: GPU flags allowed",
			backend:     "llama.cpp",
			flags:       []string{"-ngl", "99", "--main-gpu", "0"},
			expectError: false,
			description: "GPU-related flags should be allowed",
		},
		{
			name:        "llama.cpp: sampling flags allowed",
			backend:     "llama.cpp",
			flags:       []string{"--temp", "0.7", "--top-p", "0.9", "--seed", "42"},
			expectError: false,
			description: "Sampling flags should be allowed",
		},
		{
			name:    "llama.cpp: real-world flags from issue 515",
			backend: "llama.cpp",
			flags: []string{
				"--n-gpu-layers", "99",
				"--jinja",
				"--top-p", "0.8",
				"--top-k", "20",
				"--temp", "0.7",
				"--min-p", "0.0",
				"--presence-penalty", "1.5",
				"--no-mmap",
				"--flash-attn",
				"--cache-type-k", "q8_0",
				"--cache-type-v", "q8_0",
			},
			expectError: false,
			description: "Real-world flags from GitHub issue 515 should be allowed",
		},

		// Tests for vLLM backend with allowlist
		{
			name:        "vllm: valid allowed flags",
			backend:     "vllm",
			flags:       []string{"--tensor-parallel-size", "2", "--max-model-len", "4096"},
			expectError: false,
			description: "Allowed vLLM flags should pass",
		},
		{
			name:        "vllm: reject non-allowed flag",
			backend:     "vllm",
			flags:       []string{"--output-file", "test.log"},
			expectError: true,
			description: "Non-allowed flags should be rejected for vLLM",
		},
		{
			name:        "vllm: short flags allowed",
			backend:     "vllm",
			flags:       []string{"-tp", "2", "-q", "awq"},
			expectError: false,
			description: "Short vLLM flags should be allowed",
		},

		// Tests for unknown backend
		{
			name:        "unknown backend: valid flags without paths",
			backend:     "unknown-backend",
			flags:       []string{"--verbose", "--debug", "--threads", "4"},
			expectError: true,
			description: "Unknown backend should reject all flags (no allowlist)",
		},

		// Path safety tests (defense-in-depth)
		{
			name:        "llama.cpp: reject relative path with parent directory",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "../file.txt"},
			expectError: true,
			description: "Relative paths with ../ should be rejected",
		},
		{
			name:        "llama.cpp: reject relative path with current directory",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "./config.yaml"},
			expectError: true,
			description: "Relative paths with ./ should be rejected",
		},
		{
			name:        "llama.cpp: reject Windows-style path with backslash",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "C:\\Users\\file.txt"},
			expectError: true,
			description: "Windows-style paths with backslash should be rejected",
		},
		{
			name:        "llama.cpp: reject UNC network path",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "\\\\server\\share\\file.txt"},
			expectError: true,
			description: "UNC network paths should be rejected",
		},
		{
			name:        "llama.cpp: reject URL with http",
			backend:     "llama.cpp",
			flags:       []string{"--threads", "http://example.com/api"},
			expectError: true,
			description: "URLs should be rejected (conservative approach)",
		},
		{
			name:        "llama.cpp: valid flag with special characters except slash",
			backend:     "llama.cpp",
			flags:       []string{"--temp", "0.7", "--seed", "42"},
			expectError: false,
			description: "Flags with dots and numbers (no slash) should pass",
		},

		// Flag injection tests (smuggling flags via = separator)
		{
			name:        "llama.cpp: reject long flag injection via equals",
			backend:     "llama.cpp",
			flags:       []string{"--seed=--log-file=container-to-host.log"},
			expectError: true,
			description: "Smuggled long flags via = separator should be rejected",
		},
		{
			name:        "llama.cpp: reject short flag injection via equals",
			backend:     "llama.cpp",
			flags:       []string{"--seed=-l"},
			expectError: true,
			description: "Smuggled short flags via = separator should be rejected",
		},
		{
			name:        "llama.cpp: reject dash-only value via equals",
			backend:     "llama.cpp",
			flags:       []string{"--seed=-"},
			expectError: true,
			description: "Single dash as value should be rejected",
		},
		{
			name:        "llama.cpp: reject dash-dot value via equals",
			backend:     "llama.cpp",
			flags:       []string{"--temp=-.5"},
			expectError: true,
			description: "Dash followed by non-digit should be rejected",
		},
		{
			name:        "llama.cpp: allow negative integer via equals",
			backend:     "llama.cpp",
			flags:       []string{"--threads=-1"},
			expectError: false,
			description: "Negative integer values should be allowed",
		},
		{
			name:        "llama.cpp: allow negative float via equals",
			backend:     "llama.cpp",
			flags:       []string{"--temp=-0.5"},
			expectError: false,
			description: "Negative float values should be allowed",
		},
		{
			name:        "llama.cpp: allow zero via equals",
			backend:     "llama.cpp",
			flags:       []string{"--seed=0"},
			expectError: false,
			description: "Zero value should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuntimeFlags(tt.backend, tt.flags)

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

func TestValidateRuntimeFlags_ErrorMessages(t *testing.T) {
	// Test that allowlist error messages are helpful
	t.Run("allowlist rejection message", func(t *testing.T) {
		err := ValidateRuntimeFlags("llama.cpp", []string{"--log-file", "test.log"})
		if err == nil {
			t.Fatal("Expected error but got none")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "--log-file") {
			t.Errorf("Error message should contain the offending flag, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "not allowed") {
			t.Errorf("Error message should explain rejection, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "llama.cpp") {
			t.Errorf("Error message should mention the backend, got: %s", errMsg)
		}
	})

	// Test that path safety error messages are helpful
	t.Run("path rejection message", func(t *testing.T) {
		err := ValidateRuntimeFlags("llama.cpp", []string{"--threads", "/var/log/test.log"})
		if err == nil {
			t.Fatal("Expected error but got none")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "/var/log/test.log") {
			t.Errorf("Error message should contain the offending value, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "paths are not allowed") {
			t.Errorf("Error message should explain why it failed, got: %s", errMsg)
		}
	})
}

func TestValidatePathSafety(t *testing.T) {
	tests := []struct {
		name        string
		flags       []string
		expectError bool
	}{
		{
			name:        "no paths",
			flags:       []string{"--verbose", "--threads", "4"},
			expectError: false,
		},
		{
			name:        "forward slash",
			flags:       []string{"--file", "/etc/passwd"},
			expectError: true,
		},
		{
			name:        "backslash",
			flags:       []string{"--file", "C:\\Windows\\file"},
			expectError: true,
		},
		{
			name:        "relative path forward",
			flags:       []string{"../file"},
			expectError: true,
		},
		{
			name:        "relative path backward",
			flags:       []string{"..\\file"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSafety(tt.flags)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
