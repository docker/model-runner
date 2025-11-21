package commands

import (
	"bufio"
	"strings"
	"testing"

	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func TestReadMultilineInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "single line input",
			input:    "hello world",
			expected: "hello world",
			wantErr:  false,
		},
		{
			name:     "single line with triple quotes",
			input:    `"""hello world"""`,
			expected: `"""hello world"""`,
			wantErr:  false,
		},
		{
			name: "multiline input with double quotes",
			input: `"""tell
me
a
joke"""`,
			expected: `"""tell
me
a
joke"""`,
			wantErr: false,
		},
		{
			name: "multiline input with single quotes",
			input: `'''tell
me
a
joke'''`,
			expected: `'''tell
me
a
joke'''`,
			wantErr: false,
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
			wantErr:  true, // EOF should be treated as an error
		},
		{
			name: "multiline with empty lines",
			input: `"""first line

third line"""`,
			expected: `"""first line

third line"""`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock command for testing
			cmd := &cobra.Command{}

			// Create a scanner from the test input
			scanner := bufio.NewScanner(strings.NewReader(tt.input))

			// Capture output to avoid printing during tests
			var output strings.Builder
			cmd.SetOut(&output)

			result, err := readMultilineInput(cmd, scanner)

			if (err != nil) != tt.wantErr {
				t.Errorf("readMultilineInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("readMultilineInput() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestReadMultilineInputUnclosed(t *testing.T) {
	// Test unclosed multiline input (should return error)
	input := `"""unclosed multiline`
	cmd := &cobra.Command{}
	var output strings.Builder
	cmd.SetOut(&output)

	scanner := bufio.NewScanner(strings.NewReader(input))

	_, err := readMultilineInput(cmd, scanner)
	if err == nil {
		t.Error("readMultilineInput() should return error for unclosed multiline input")
	}

	if !strings.Contains(err.Error(), "unclosed multiline input") {
		t.Errorf("readMultilineInput() error should mention unclosed multiline input, got: %v", err)
	}
}

func TestRunCmdDetachFlag(t *testing.T) {
	// Create the run command
	cmd := newRunCmd()

	// Verify the --detach flag exists
	detachFlag := cmd.Flags().Lookup("detach")
	if detachFlag == nil {
		t.Fatal("--detach flag not found")
	}

	// Verify the shorthand flag exists
	detachFlagShort := cmd.Flags().ShorthandLookup("d")
	if detachFlagShort == nil {
		t.Fatal("-d shorthand flag not found")
	}

	// Verify the default value is false
	if detachFlag.DefValue != "false" {
		t.Errorf("Expected default detach value to be 'false', got '%s'", detachFlag.DefValue)
	}

	// Verify the flag type
	if detachFlag.Value.Type() != "bool" {
		t.Errorf("Expected detach flag type to be 'bool', got '%s'", detachFlag.Value.Type())
	}

	// Test setting the flag value
	err := cmd.Flags().Set("detach", "true")
	if err != nil {
		t.Errorf("Failed to set detach flag: %v", err)
	}

	// Verify the value was set
	detachValue, err := cmd.Flags().GetBool("detach")
	if err != nil {
		t.Errorf("Failed to get detach flag value: %v", err)
	}

	if !detachValue {
		t.Errorf("Expected detach flag value to be true, got false")
	}
}

// TestRunModelNameNormalization verifies that model names are normalized correctly
// in the run command to ensure consistency with how models are stored after pulling
func TestRunModelNameNormalization(t *testing.T) {
	tests := []struct {
		name                   string
		userProvidedModelName  string
		expectedNormalizedName string
		description            string
	}{
		{
			name:                   "simple model name without namespace",
			userProvidedModelName:  "llama3",
			expectedNormalizedName: "ai/llama3:latest",
			description:            "When user runs 'docker model run llama3', it should be normalized to 'ai/llama3:latest'",
		},
		{
			name:                   "model name with tag but no namespace",
			userProvidedModelName:  "llama3:8b",
			expectedNormalizedName: "ai/llama3:8b",
			description:            "When user runs 'docker model run llama3:8b', it should be normalized to 'ai/llama3:8b'",
		},
		{
			name:                   "model name with explicit namespace",
			userProvidedModelName:  "myorg/llama3",
			expectedNormalizedName: "myorg/llama3:latest",
			description:            "When user runs 'docker model run myorg/llama3', it should preserve the namespace",
		},
		{
			name:                   "model name with ai namespace already set",
			userProvidedModelName:  "ai/llama3",
			expectedNormalizedName: "ai/llama3:latest",
			description:            "When user runs 'docker model run ai/llama3', it should remain as 'ai/llama3:latest'",
		},
		{
			name:                   "fully qualified model name",
			userProvidedModelName:  "ai/llama3:latest",
			expectedNormalizedName: "ai/llama3:latest",
			description:            "When user runs 'docker model run ai/llama3:latest', it should remain unchanged",
		},
		{
			name:                   "model name with custom org and tag",
			userProvidedModelName:  "myorg/llama3:v2",
			expectedNormalizedName: "myorg/llama3:v2",
			description:            "When user runs 'docker model run myorg/llama3:v2', it should remain unchanged",
		},
		{
			name:                   "huggingface model",
			userProvidedModelName:  "hf.co/meta-llama/Llama-3-8B",
			expectedNormalizedName: "hf.co/meta-llama/llama-3-8b:latest",
			description:            "HuggingFace models should be lowercased and tagged with :latest",
		},
		{
			name:                   "registry prefixed model",
			userProvidedModelName:  "registry.example.com/mymodel",
			expectedNormalizedName: "registry.example.com/mymodel:latest",
			description:            "Registry-prefixed models should only get :latest tag added",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the normalization function produces the expected output
			normalizedName := dmrm.NormalizeModelName(tt.userProvidedModelName)

			if normalizedName != tt.expectedNormalizedName {
				t.Errorf("NormalizeModelName(%q) = %q, want %q\nDescription: %s",
					tt.userProvidedModelName,
					normalizedName,
					tt.expectedNormalizedName,
					tt.description)
			}
		})
	}
}

// TestRunModelNameNormalizationConsistency verifies that the run command
// uses the same normalization as the pull command, ensuring that:
// 1. A model pulled as "docker model pull mymodel" creates "ai/mymodel:latest"
// 2. The same model can be run as "docker model run mymodel" (without ai/ prefix)
func TestRunModelNameNormalizationConsistency(t *testing.T) {
	testCases := []struct {
		name                      string
		userInputForPull          string
		userInputForRun           string
		expectedInternalReference string
		description               string
	}{
		{
			name:                      "pull and run without namespace",
			userInputForPull:          "gemma3",
			userInputForRun:           "gemma3",
			expectedInternalReference: "ai/gemma3:latest",
			description:               "User pulls 'gemma3' and runs 'gemma3' - both should resolve to 'ai/gemma3:latest'",
		},
		{
			name:                      "pull with namespace and run without namespace",
			userInputForPull:          "ai/gemma3",
			userInputForRun:           "gemma3",
			expectedInternalReference: "ai/gemma3:latest",
			description:               "User pulls 'ai/gemma3' and runs 'gemma3' - both should resolve to 'ai/gemma3:latest'",
		},
		{
			name:                      "pull without namespace and run with namespace",
			userInputForPull:          "gemma3",
			userInputForRun:           "ai/gemma3",
			expectedInternalReference: "ai/gemma3:latest",
			description:               "User pulls 'gemma3' and runs 'ai/gemma3' - both should resolve to 'ai/gemma3:latest'",
		},
		{
			name:                      "pull and run with tag",
			userInputForPull:          "gemma3:2b",
			userInputForRun:           "gemma3:2b",
			expectedInternalReference: "ai/gemma3:2b",
			description:               "User pulls 'gemma3:2b' and runs 'gemma3:2b' - both should resolve to 'ai/gemma3:2b'",
		},
		{
			name:                      "custom org is preserved",
			userInputForPull:          "myorg/gemma3",
			userInputForRun:           "myorg/gemma3",
			expectedInternalReference: "myorg/gemma3:latest",
			description:               "User pulls 'myorg/gemma3' and runs 'myorg/gemma3' - both should resolve to 'myorg/gemma3:latest'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate what happens during pull (model name is normalized before storage)
			normalizedPullName := dmrm.NormalizeModelName(tc.userInputForPull)

			// Simulate what should happen during run (model name is normalized before lookup)
			normalizedRunName := dmrm.NormalizeModelName(tc.userInputForRun)

			// Both should normalize to the same internal reference
			if normalizedPullName != tc.expectedInternalReference || normalizedRunName != tc.expectedInternalReference {
				t.Errorf("Normalization failed for test case %q:\n  Pull input: %q -> Got %q, Want %q\n  Run input:  %q -> Got %q, Want %q\n  Description: %s",
					tc.name,
					tc.userInputForPull, normalizedPullName, tc.expectedInternalReference,
					tc.userInputForRun, normalizedRunName, tc.expectedInternalReference,
					tc.description)
			}
		})
	}
}
