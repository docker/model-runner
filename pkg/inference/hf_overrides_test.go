package inference

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHFOverrides_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   HFOverrides
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name:    "valid architectures array",
			input:   HFOverrides{"architectures": []interface{}{"Qwen3ForSequenceClassification"}},
			wantErr: false,
		},
		{
			name:    "valid multiple architectures",
			input:   HFOverrides{"architectures": []interface{}{"GteNewModel", "GemmaForSequenceClassification"}},
			wantErr: false,
		},
		{
			name:    "valid boolean value",
			input:   HFOverrides{"is_original_qwen3_reranker": true},
			wantErr: false,
		},
		{
			name:    "valid string value",
			input:   HFOverrides{"model_type": "bert"},
			wantErr: false,
		},
		{
			name:    "valid number value",
			input:   HFOverrides{"num_labels": float64(2)},
			wantErr: false,
		},
		{
			name:    "valid null value",
			input:   HFOverrides{"some_field": nil},
			wantErr: false,
		},
		{
			name: "valid complex example from user",
			input: HFOverrides{
				"architectures":              []interface{}{"Qwen3ForSequenceClassification"},
				"classifier_from_token":      []interface{}{"no", "yes"},
				"is_original_qwen3_reranker": true,
			},
			wantErr: false,
		},
		{
			name:    "valid key starting with underscore",
			input:   HFOverrides{"_private_config": "value"},
			wantErr: false,
		},
		{
			name:    "valid key with numbers",
			input:   HFOverrides{"layer2_config": "value"},
			wantErr: false,
		},
		{
			name:    "valid empty map",
			input:   HFOverrides{},
			wantErr: false,
		},

		// Invalid key tests - command injection attempts
		{
			name:    "SECURITY: reject key with dash (potential flag injection)",
			input:   HFOverrides{"--malicious-flag": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key that looks like a flag",
			input:   HFOverrides{"-v": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key starting with number",
			input:   HFOverrides{"123key": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with spaces",
			input:   HFOverrides{"key with spaces": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with quotes",
			input:   HFOverrides{"key\"injection": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with semicolon (shell command separator)",
			input:   HFOverrides{"key;rm -rf /": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with backticks (command substitution)",
			input:   HFOverrides{"key`whoami`": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with dollar sign (variable expansion)",
			input:   HFOverrides{"key$HOME": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with pipe (command piping)",
			input:   HFOverrides{"key|cat /etc/passwd": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with ampersand (background execution)",
			input:   HFOverrides{"key&malicious": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with newline",
			input:   HFOverrides{"key\ninjection": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with carriage return",
			input:   HFOverrides{"key\rinjection": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with parentheses (subshell)",
			input:   HFOverrides{"key$(whoami)": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with curly braces",
			input:   HFOverrides{"key{evil}": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject key with square brackets",
			input:   HFOverrides{"key[0]": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject empty key",
			input:   HFOverrides{"": "value"},
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},

		// Valid nested object tests
		{
			name:    "valid nested object with safe keys",
			input:   HFOverrides{"nested": map[string]interface{}{"valid_key": "data"}},
			wantErr: false,
		},
		{
			name:    "valid array with nested object",
			input:   HFOverrides{"array": []interface{}{map[string]interface{}{"valid_key": "data"}}},
			wantErr: false,
		},
		{
			name:    "valid deeply nested structure",
			input:   HFOverrides{"deep": map[string]interface{}{"level1": map[string]interface{}{"level2": "value"}}},
			wantErr: false,
		},
		{
			name:    "valid rope_scaling config (real HuggingFace example)",
			input:   HFOverrides{"rope_scaling": map[string]interface{}{"type": "linear", "factor": float64(2.0)}},
			wantErr: false,
		},
		{
			name: "valid complex nested config",
			input: HFOverrides{
				"architectures": []interface{}{"Model"},
				"config": map[string]interface{}{
					"option1": true,
					"option2": "value",
					"nested": map[string]interface{}{
						"deep_option": float64(42),
					},
				},
			},
			wantErr: false,
		},

		// SECURITY: Invalid nested key tests
		{
			name:    "SECURITY: reject nested object with dash in key",
			input:   HFOverrides{"nested": map[string]interface{}{"--malicious-flag": "data"}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
		{
			name:    "SECURITY: reject deeply nested invalid key",
			input:   HFOverrides{"deep": map[string]interface{}{"level1": map[string]interface{}{"--bad-key": "value"}}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
		{
			name:    "SECURITY: reject array with nested object containing bad key",
			input:   HFOverrides{"array": []interface{}{map[string]interface{}{"key;injection": "data"}}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
		{
			name:    "SECURITY: reject nested key with shell injection",
			input:   HFOverrides{"config": map[string]interface{}{"key$(whoami)": "value"}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
		{
			name:    "SECURITY: reject nested key starting with number",
			input:   HFOverrides{"config": map[string]interface{}{"123key": "value"}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
		{
			name:    "SECURITY: reject empty nested key",
			input:   HFOverrides{"config": map[string]interface{}{"": "value"}},
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestHFOverrides_JSONParsing(t *testing.T) {
	// Test that JSON parsing followed by validation works correctly
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid JSON from user example",
			json:    `{"architectures": ["Qwen3ForSequenceClassification"],"classifier_from_token": ["no", "yes"],"is_original_qwen3_reranker": true}`,
			wantErr: false,
		},
		{
			name:    "valid JSON with GteNewModel",
			json:    `{"architectures": ["GteNewModel"]}`,
			wantErr: false,
		},
		{
			name:    "valid JSON with multiple architectures",
			json:    `{"architectures": ["GemmaForSequenceClassification", "Emu3ForConditionalGeneration"]}`,
			wantErr: false,
		},
		{
			name:    "valid JSON with nested object (rope_scaling)",
			json:    `{"rope_scaling": {"type": "linear", "factor": 2.0}}`,
			wantErr: false,
		},
		{
			name:    "valid JSON with deeply nested config",
			json:    `{"config": {"nested": {"deep": "value"}}}`,
			wantErr: false,
		},
		{
			name:    "SECURITY: reject JSON with flag-like key",
			json:    `{"--hf-overrides": "injection"}`,
			wantErr: true,
			errMsg:  "invalid hf_overrides key",
		},
		{
			name:    "SECURITY: reject JSON with nested object containing bad key",
			json:    `{"config": {"--bad-key": "value"}}`,
			wantErr: true,
			errMsg:  "invalid hf_overrides nested key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hfo HFOverrides
			if err := json.Unmarshal([]byte(tt.json), &hfo); err != nil {
				t.Fatalf("json.Unmarshal() failed: %v", err)
			}

			err := hfo.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestHFOverrides_JSONSerialization(t *testing.T) {
	// Test that valid HFOverrides can be serialized back to JSON safely
	tests := []struct {
		name  string
		input HFOverrides
	}{
		{
			name:  "simple architectures",
			input: HFOverrides{"architectures": []interface{}{"TestModel"}},
		},
		{
			name: "complex valid input",
			input: HFOverrides{
				"architectures":              []interface{}{"Qwen3ForSequenceClassification"},
				"classifier_from_token":      []interface{}{"no", "yes"},
				"is_original_qwen3_reranker": true,
				"num_labels":                 float64(2),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate first
			if err := tt.input.Validate(); err != nil {
				t.Fatalf("Validate() failed: %v", err)
			}

			// Serialize
			result, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() failed: %v", err)
			}

			// Verify it's valid JSON by parsing it back
			var parsed map[string]interface{}
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Errorf("json.Unmarshal() of serialized result failed: %v", err)
			}
		})
	}
}

func TestHFOverrides_KeyRegexBoundaries(t *testing.T) {
	// Test boundary cases for the key regex
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"single letter", "a", false},
		{"single underscore", "_", false},
		{"uppercase letter", "A", false},
		{"letter then number", "a1", false},
		{"underscore then number", "_1", false},
		{"underscore then letter", "_a", false},
		{"all underscores", "___", false},
		{"mixed case", "AbCdEf", false},
		{"typical snake_case", "model_type", false},
		{"typical camelCase", "modelType", false},
		{"number only", "1", true},
		{"starts with number", "1a", true},
		{"contains dot", "model.type", true},
		{"contains colon", "model:type", true},
		{"contains slash", "model/type", true},
		{"contains backslash", "model\\type", true},
		{"unicode letter", "模型", true}, // Chinese characters should be rejected
		{"emoji", "🚀", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hfo := HFOverrides{tt.key: "value"}
			err := hfo.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() for key %q: error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}
