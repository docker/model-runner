package commands

import (
	"testing"
)

func TestInstallRunnerHostFlag(t *testing.T) {
	// Create the install-runner command
	cmd := newInstallRunner()

	// Verify the --host flag exists
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
	}

	// Verify the default value
	if hostFlag.DefValue != "127.0.0.1" {
		t.Errorf("Expected default host value to be '127.0.0.1', got '%s'", hostFlag.DefValue)
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
	expectedFlags := []string{"port", "host", "gpu", "do-not-track", "ollama"}
	for _, flagName := range expectedFlags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("Expected flag '--%s' not found", flagName)
		}
	}
}

func TestInstallRunnerOllamaFlag(t *testing.T) {
	cmd := newInstallRunner()

	// Verify the --ollama flag exists
	ollamaFlag := cmd.Flags().Lookup("ollama")
	if ollamaFlag == nil {
		t.Fatal("--ollama flag not found")
	}

	// Verify the default value
	if ollamaFlag.DefValue != "false" {
		t.Errorf("Expected default ollama value to be 'false', got '%s'", ollamaFlag.DefValue)
	}

	// Verify the flag type
	if ollamaFlag.Value.Type() != "bool" {
		t.Errorf("Expected ollama flag type to be 'bool', got '%s'", ollamaFlag.Value.Type())
	}

	// Test setting the flag value
	err := cmd.Flags().Set("ollama", "true")
	if err != nil {
		t.Errorf("Failed to set ollama flag: %v", err)
	}

	// Verify the value was set
	ollamaValue, err := cmd.Flags().GetBool("ollama")
	if err != nil {
		t.Errorf("Failed to get ollama flag value: %v", err)
	}
	if !ollamaValue {
		t.Error("Expected ollama value to be true")
	}
}

func TestInstallRunnerGPUFlag(t *testing.T) {
	cmd := newInstallRunner()

	// Verify the --gpu flag exists
	gpuFlag := cmd.Flags().Lookup("gpu")
	if gpuFlag == nil {
		t.Fatal("--gpu flag not found")
	}

	// Test setting gpu to rocm
	err := cmd.Flags().Set("gpu", "rocm")
	if err != nil {
		t.Errorf("Failed to set gpu flag to 'rocm': %v", err)
	}

	// Verify the value was set
	gpuValue, err := cmd.Flags().GetString("gpu")
	if err != nil {
		t.Errorf("Failed to get gpu flag value: %v", err)
	}
	if gpuValue != "rocm" {
		t.Errorf("Expected gpu value to be 'rocm', got '%s'", gpuValue)
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
