package commands

import (
	"testing"
)

func TestConfigureCmdReasoningBudgetFlag(t *testing.T) {
	// Create the configure command
	cmd := newConfigureCmd()

	// Verify the --reasoning-budget flag exists
	reasoningBudgetFlag := cmd.Flags().Lookup("reasoning-budget")
	if reasoningBudgetFlag == nil {
		t.Fatal("--reasoning-budget flag not found")
	}

	// Verify the default value is empty (nil pointer)
	if reasoningBudgetFlag.DefValue != "" {
		t.Errorf("Expected default reasoning-budget value to be '' (nil), got '%s'", reasoningBudgetFlag.DefValue)
	}

	// Verify the flag type
	if reasoningBudgetFlag.Value.Type() != "int32" {
		t.Errorf("Expected reasoning-budget flag type to be 'int32', got '%s'", reasoningBudgetFlag.Value.Type())
	}
}

func TestConfigureCmdReasoningBudgetFlagChanged(t *testing.T) {
	tests := []struct {
		name          string
		setValue      string
		expectChanged bool
		expectedValue string
	}{
		{
			name:          "flag not set - should not be changed",
			setValue:      "",
			expectChanged: false,
			expectedValue: "",
		},
		{
			name:          "flag set to 0 (disable reasoning) - should be changed",
			setValue:      "0",
			expectChanged: true,
			expectedValue: "0",
		},
		{
			name:          "flag set to -1 (unlimited) - should be changed",
			setValue:      "-1",
			expectChanged: true,
			expectedValue: "-1",
		},
		{
			name:          "flag set to positive value - should be changed",
			setValue:      "1024",
			expectChanged: true,
			expectedValue: "1024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh configure command for each test
			cmd := newConfigureCmd()

			// Only set the flag if setValue is not empty
			if tt.setValue != "" {
				err := cmd.Flags().Set("reasoning-budget", tt.setValue)
				if err != nil {
					t.Fatalf("Failed to set reasoning-budget flag: %v", err)
				}
			}

			// Check if the flag was marked as changed
			isChanged := cmd.Flags().Changed("reasoning-budget")
			if isChanged != tt.expectChanged {
				t.Errorf("Expected Changed() = %v, got %v", tt.expectChanged, isChanged)
			}

			// Verify the value using String() method
			flag := cmd.Flags().Lookup("reasoning-budget")
			value := flag.Value.String()
			if value != tt.expectedValue {
				t.Errorf("Expected value = %s, got %s", tt.expectedValue, value)
			}
		})
	}
}

func TestConfigureCmdHfOverridesFlag(t *testing.T) {
	// Create the configure command
	cmd := newConfigureCmd()

	// Verify the --hf_overrides flag exists
	hfOverridesFlag := cmd.Flags().Lookup("hf_overrides")
	if hfOverridesFlag == nil {
		t.Fatal("--hf_overrides flag not found")
	}

	// Verify the default value is empty
	if hfOverridesFlag.DefValue != "" {
		t.Errorf("Expected default hf_overrides value to be empty, got '%s'", hfOverridesFlag.DefValue)
	}

	// Verify the flag type
	if hfOverridesFlag.Value.Type() != "string" {
		t.Errorf("Expected hf_overrides flag type to be 'string', got '%s'", hfOverridesFlag.Value.Type())
	}
}

func TestConfigureCmdContextSizeFlag(t *testing.T) {
	// Create the configure command
	cmd := newConfigureCmd()

	// Verify the --context-size flag exists
	contextSizeFlag := cmd.Flags().Lookup("context-size")
	if contextSizeFlag == nil {
		t.Fatal("--context-size flag not found")
	}

	// Verify the default value is empty (nil pointer)
	if contextSizeFlag.DefValue != "" {
		t.Errorf("Expected default context-size value to be '' (nil), got '%s'", contextSizeFlag.DefValue)
	}

	// Test setting the flag value
	err := cmd.Flags().Set("context-size", "8192")
	if err != nil {
		t.Errorf("Failed to set context-size flag: %v", err)
	}

	// Verify the value was set using String() method
	contextSizeValue := contextSizeFlag.Value.String()
	if contextSizeValue != "8192" {
		t.Errorf("Expected context-size flag value to be '8192', got '%s'", contextSizeValue)
	}
}

func TestConfigureCmdSpeculativeFlags(t *testing.T) {
	cmd := newConfigureCmd()

	// Test speculative-draft-model flag
	draftModelFlag := cmd.Flags().Lookup("speculative-draft-model")
	if draftModelFlag == nil {
		t.Fatal("--speculative-draft-model flag not found")
	}

	// Test speculative-num-tokens flag
	numTokensFlag := cmd.Flags().Lookup("speculative-num-tokens")
	if numTokensFlag == nil {
		t.Fatal("--speculative-num-tokens flag not found")
	}

	// Test speculative-min-acceptance-rate flag
	minAcceptanceRateFlag := cmd.Flags().Lookup("speculative-min-acceptance-rate")
	if minAcceptanceRateFlag == nil {
		t.Fatal("--speculative-min-acceptance-rate flag not found")
	}
}

func TestConfigureCmdModeFlag(t *testing.T) {
	// Create the configure command
	cmd := newConfigureCmd()

	// Verify the --mode flag exists
	modeFlag := cmd.Flags().Lookup("mode")
	if modeFlag == nil {
		t.Fatal("--mode flag not found")
	}

	// Verify the default value is empty
	if modeFlag.DefValue != "" {
		t.Errorf("Expected default mode value to be empty, got '%s'", modeFlag.DefValue)
	}

	// Verify the flag type
	if modeFlag.Value.Type() != "string" {
		t.Errorf("Expected mode flag type to be 'string', got '%s'", modeFlag.Value.Type())
	}
}

func TestConfigureCmdThinkFlag(t *testing.T) {
	// Create the configure command
	cmd := newConfigureCmd()

	// Verify the --think flag exists
	thinkFlag := cmd.Flags().Lookup("think")
	if thinkFlag == nil {
		t.Fatal("--think flag not found")
	}

	// Verify the default value is empty
	if thinkFlag.DefValue != "" {
		t.Errorf("Expected default think value to be empty, got '%s'", thinkFlag.DefValue)
	}

	// Verify the flag type
	if thinkFlag.Value.Type() != "string" {
		t.Errorf("Expected think flag type to be 'string', got '%s'", thinkFlag.Value.Type())
	}
}

// TestThinkAndReasoningBudgetMutualExclusivity verifies that --think and --reasoning-budget
// cannot be used together
func TestThinkAndReasoningBudgetMutualExclusivity(t *testing.T) {
	tests := []struct {
		name            string
		think           string
		reasoningBudget *int32
		expectError     bool
		errorContains   string
	}{
		{
			name:            "only think flag set",
			think:           "high",
			reasoningBudget: nil,
			expectError:     false,
		},
		{
			name:            "only reasoning-budget flag set",
			think:           "",
			reasoningBudget: ptr(1024),
			expectError:     false,
		},
		{
			name:            "neither flag set",
			think:           "",
			reasoningBudget: nil,
			expectError:     false,
		},
		{
			name:            "both flags set - should error",
			think:           "high",
			reasoningBudget: ptr(1024),
			expectError:     true,
			errorContains:   "mutually exclusive",
		},
		{
			name:            "both flags set with think=false - should error",
			think:           "false",
			reasoningBudget: ptr(0),
			expectError:     true,
			errorContains:   "mutually exclusive",
		},
		{
			name:            "both flags set with think=medium - should error",
			think:           "medium",
			reasoningBudget: ptr(-1),
			expectError:     true,
			errorContains:   "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := ConfigureFlags{
				Think:           tt.think,
				ReasoningBudget: tt.reasoningBudget,
			}

			_, err := flags.BuildConfigureRequest("test-model")

			if tt.expectError {
				if err == nil {
					t.Error("Expected error when both --think and --reasoning-budget are set, but got nil")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
