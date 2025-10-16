package commands

import (
	"bufio"
	"strings"
	"testing"

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

func TestRunCmdServerConnectionFlags(t *testing.T) {
	// Create the run command
	cmd := newRunCmd()

	// Test --host flag
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
	}
	if hostFlag.Value.Type() != "string" {
		t.Errorf("Expected host flag type to be 'string', got '%s'", hostFlag.Value.Type())
	}

	// Test --port flag
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag not found")
	}
	if portFlag.Value.Type() != "int" {
		t.Errorf("Expected port flag type to be 'int', got '%s'", portFlag.Value.Type())
	}

	// Test --url flag
	urlFlag := cmd.Flags().Lookup("url")
	if urlFlag == nil {
		t.Fatal("--url flag not found")
	}
	if urlFlag.Value.Type() != "string" {
		t.Errorf("Expected url flag type to be 'string', got '%s'", urlFlag.Value.Type())
	}

	// Test --url-alias flag
	urlAliasFlag := cmd.Flags().Lookup("url-alias")
	if urlAliasFlag == nil {
		t.Fatal("--url-alias flag not found")
	}
	if urlAliasFlag.Value.Type() != "string" {
		t.Errorf("Expected url-alias flag type to be 'string', got '%s'", urlAliasFlag.Value.Type())
	}
}

func TestListCmdServerConnectionFlags(t *testing.T) {
	// Create the list command
	cmd := newListCmd()

	// Test --host flag
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
	}
	if hostFlag.Value.Type() != "string" {
		t.Errorf("Expected host flag type to be 'string', got '%s'", hostFlag.Value.Type())
	}

	// Test --port flag
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag not found")
	}
	if portFlag.Value.Type() != "int" {
		t.Errorf("Expected port flag type to be 'int', got '%s'", portFlag.Value.Type())
	}

	// Test --url flag
	urlFlag := cmd.Flags().Lookup("url")
	if urlFlag == nil {
		t.Fatal("--url flag not found")
	}
	if urlFlag.Value.Type() != "string" {
		t.Errorf("Expected url flag type to be 'string', got '%s'", urlFlag.Value.Type())
	}

	// Test --url-alias flag
	urlAliasFlag := cmd.Flags().Lookup("url-alias")
	if urlAliasFlag == nil {
		t.Fatal("--url-alias flag not found")
	}
	if urlAliasFlag.Value.Type() != "string" {
		t.Errorf("Expected url-alias flag type to be 'string', got '%s'", urlAliasFlag.Value.Type())
	}
}

func TestPullCmdServerConnectionFlags(t *testing.T) {
	// Create the pull command
	cmd := newPullCmd()

	// Test --host flag
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
	}
	if hostFlag.Value.Type() != "string" {
		t.Errorf("Expected host flag type to be 'string', got '%s'", hostFlag.Value.Type())
	}

	// Test --port flag
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag not found")
	}
	if portFlag.Value.Type() != "int" {
		t.Errorf("Expected port flag type to be 'int', got '%s'", portFlag.Value.Type())
	}
}

func TestPushCmdServerConnectionFlags(t *testing.T) {
	// Create the push command
	cmd := newPushCmd()

	// Test --host flag
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("--host flag not found")
	}
	if hostFlag.Value.Type() != "string" {
		t.Errorf("Expected host flag type to be 'string', got '%s'", hostFlag.Value.Type())
	}

	// Test --port flag
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag not found")
	}
	if portFlag.Value.Type() != "int" {
		t.Errorf("Expected port flag type to be 'int', got '%s'", portFlag.Value.Type())
	}
}
