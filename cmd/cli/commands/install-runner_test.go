package commands

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
)

func TestInstallRunnerHostFlag(t *testing.T) {
	// Create the install-runner command
	cmd := newInstallRunner()

	// Verify the --host flag exists
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
		return // unreachable but satisfies staticcheck SA5011
	}

	// Get values to avoid potential nil dereference flagged by linter
	defValue := hostFlag.DefValue

	// Verify the default value
	if defValue != "127.0.0.1" {
		t.Errorf("Expected default host value to be '127.0.0.1', got '%s'", defValue)
	}

	// Verify the flag type
	if hostFlag.Value.Type() != "string" {
		t.Errorf("Expected host flag type to be 'string', got '%s'", hostFlag.Value.Type())
	}

	// Test setting the flag value
	testCases := []struct {
		name  string
		value string
	}{
		{"localhost", "127.0.0.1"},
		{"all interfaces", "0.0.0.0"},
		{"specific IP", "192.168.1.100"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the command for each test
			cmd := newInstallRunner()
			err := cmd.Flags().Set("host", tc.value)
			if err != nil {
				t.Errorf("Failed to set host flag to '%s': %v", tc.value, err)
			}

			// Verify the value was set
			hostValue, err := cmd.Flags().GetString("host")
			if err != nil {
				t.Errorf("Failed to get host flag value: %v", err)
			}
			if hostValue != tc.value {
				t.Errorf("Expected host value to be '%s', got '%s'", tc.value, hostValue)
			}
		})
	}
}

func TestInstallRunnerCommandFlags(t *testing.T) {
	cmd := newInstallRunner()

	// Verify all expected flags exist
	expectedFlags := []string{"port", "host", "gpu", "backend", "do-not-track"}
	for _, flagName := range expectedFlags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("Expected flag '--%s' not found", flagName)
		}
	}
}

func TestInstallRunnerBackendFlag(t *testing.T) {
	cmd := newInstallRunner()

	// Verify the --backend flag exists
	backendFlag := cmd.Flags().Lookup("backend")
	if backendFlag == nil {
		t.Fatal("--backend flag not found")
		return // unreachable but satisfies staticcheck SA5011
	}

	// Get values to avoid potential nil dereference flagged by linter
	defValue := backendFlag.DefValue

	// Verify the default value
	if defValue != "" {
		t.Errorf("Expected default backend value to be empty, got '%s'", defValue)
	}

	// Verify the flag type
	if backendFlag.Value.Type() != "string" {
		t.Errorf("Expected backend flag type to be 'string', got '%s'", backendFlag.Value.Type())
	}

	// Test setting the flag to vllm
	err := cmd.Flags().Set("backend", vllm.Name)
	if err != nil {
		t.Errorf("Failed to set backend flag: %v", err)
	}

	// Verify the value was set
	backendValue, err := cmd.Flags().GetString("backend")
	if err != nil {
		t.Errorf("Failed to get backend flag value: %v", err)
	}
	if backendValue != vllm.Name {
		t.Errorf("Expected backend value to be 'vllm', got '%s'", backendValue)
	}

	// Test setting the flag to llama.cpp
	err = cmd.Flags().Set("backend", llamacpp.Name)
	if err != nil {
		t.Errorf("Failed to set backend flag to llama.cpp: %v", err)
	}

	backendValue, err = cmd.Flags().GetString("backend")
	if err != nil {
		t.Errorf("Failed to get backend flag value: %v", err)
	}
	if backendValue != llamacpp.Name {
		t.Errorf("Expected backend value to be 'llama.cpp', got '%s'", backendValue)
	}
}

func TestInstallRunnerCommandType(t *testing.T) {
	cmd := newInstallRunner()

	// Verify command properties
	if cmd.Use != "install-runner" {
		t.Errorf("Expected command Use to be 'install-runner', got '%s'", cmd.Use)
	}

	if cmd.Short != "Install Docker Model Runner (Docker Engine only)" {
		t.Errorf("Unexpected command Short description: %s", cmd.Short)
	}

	// Verify RunE is set
	if cmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

func TestInstallRunnerValidArgsFunction(t *testing.T) {
	cmd := newInstallRunner()

	// The install-runner command should not accept any arguments
	// So ValidArgsFunction should be set to handle no arguments
	if cmd.ValidArgsFunction == nil {
		t.Error("Expected ValidArgsFunction to be set")
	}
}
