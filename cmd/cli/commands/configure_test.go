package commands

import (
	"testing"
)

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
		t.Errorf("Expected default think value to be empty (nil), got '%s'", thinkFlag.DefValue)
	}

	// Verify the flag type
	if thinkFlag.Value.Type() != "bool" {
		t.Errorf("Expected think flag type to be 'bool', got '%s'", thinkFlag.Value.Type())
	}

	// Test setting the flag to true
	err := cmd.Flags().Set("think", "true")
	if err != nil {
		t.Errorf("Failed to set think flag to true: %v", err)
	}

	// Verify the value was set
	if thinkFlag.Value.String() != "true" {
		t.Errorf("Expected think flag value to be 'true', got '%s'", thinkFlag.Value.String())
	}
}

func TestThinkFlagBehavior(t *testing.T) {
	// Helper to create bool pointer
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name           string
		thinkValue     *bool
		expectBudget   bool
		expectedBudget int32
	}{
		{
			name:         "default - not set (nil)",
			thinkValue:   nil,
			expectBudget: false,
		},
		{
			name:           "explicitly set to true (--think)",
			thinkValue:     boolPtr(true),
			expectBudget:   true,
			expectedBudget: -1,
		},
		{
			name:           "explicitly set to false (--think=false)",
			thinkValue:     boolPtr(false),
			expectBudget:   true,
			expectedBudget: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := ConfigureFlags{
				Think: tt.thinkValue,
			}

			req, err := flags.BuildConfigureRequest("test-model")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectBudget {
				// Reasoning budget should be set
				if req.LlamaCpp == nil || req.LlamaCpp.ReasoningBudget == nil {
					t.Fatal("Expected reasoning budget to be set")
				}
				if *req.LlamaCpp.ReasoningBudget != tt.expectedBudget {
					t.Errorf("Expected reasoning budget to be %d, got %d", tt.expectedBudget, *req.LlamaCpp.ReasoningBudget)
				}
			} else {
				// Reasoning budget should NOT be set
				if req.LlamaCpp != nil && req.LlamaCpp.ReasoningBudget != nil {
					t.Errorf("Expected reasoning budget to be nil when not set, got %d", *req.LlamaCpp.ReasoningBudget)
				}
			}
		})
	}
}
