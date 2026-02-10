package inference

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseKeepAlive(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    KeepAlive
		expectError bool
	}{
		{
			name:     "zero means unload immediately",
			input:    "0",
			expected: KeepAliveImmediate,
		},
		{
			name:     "negative one means never unload",
			input:    "-1",
			expected: KeepAliveForever,
		},
		{
			name:     "negative duration means never unload",
			input:    "-1m",
			expected: KeepAliveForever,
		},
		{
			name:     "5 minutes",
			input:    "5m",
			expected: KeepAlive(5 * time.Minute),
		},
		{
			name:     "1 hour",
			input:    "1h",
			expected: KeepAlive(1 * time.Hour),
		},
		{
			name:     "30 seconds",
			input:    "30s",
			expected: KeepAlive(30 * time.Second),
		},
		{
			name:     "24 hours",
			input:    "24h",
			expected: KeepAlive(24 * time.Hour),
		},
		{
			name:     "complex duration",
			input:    "1h30m",
			expected: KeepAlive(1*time.Hour + 30*time.Minute),
		},
		{
			name:        "invalid string",
			input:       "abc",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ParseKeepAlive(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
				return
			}
			if d != tt.expected {
				t.Errorf("expected %v for input %q, got %v", tt.expected, tt.input, d)
			}
		})
	}
}

func TestKeepAliveDuration(t *testing.T) {
	ka := KeepAlive(5 * time.Minute)
	if ka.Duration() != 5*time.Minute {
		t.Errorf("expected 5m, got %v", ka.Duration())
	}

	if KeepAliveForever.Duration() != -1 {
		t.Errorf("expected -1ns, got %v", KeepAliveForever.Duration())
	}

	if KeepAliveImmediate.Duration() != 0 {
		t.Errorf("expected 0, got %v", KeepAliveImmediate.Duration())
	}

	if KeepAliveDefault.Duration() != 5*time.Minute {
		t.Errorf("expected 5m, got %v", KeepAliveDefault.Duration())
	}
}

func TestKeepAliveJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    KeepAlive
		expected string
	}{
		{
			name:     "5 minutes",
			input:    KeepAlive(5 * time.Minute),
			expected: `"5m0s"`,
		},
		{
			name:     "never unload",
			input:    KeepAliveForever,
			expected: `"-1"`,
		},
	}

	for _, tt := range tests {
		t.Run("marshal "+tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(data))
			}
		})

		t.Run("roundtrip "+tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var result KeepAlive
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if result != tt.input {
				t.Errorf("roundtrip mismatch: expected %v, got %v", tt.input, result)
			}
		})
	}

	t.Run("unmarshal 0", func(t *testing.T) {
		var ka KeepAlive
		if err := json.Unmarshal([]byte(`"0"`), &ka); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ka != KeepAliveImmediate {
			t.Errorf("expected KeepAliveImmediate, got %v", ka)
		}
	})

	t.Run("unmarshal -1", func(t *testing.T) {
		var ka KeepAlive
		if err := json.Unmarshal([]byte(`"-1"`), &ka); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ka != KeepAliveForever {
			t.Errorf("expected KeepAliveForever, got %v", ka)
		}
	})

	t.Run("unmarshal invalid", func(t *testing.T) {
		var ka KeepAlive
		if err := json.Unmarshal([]byte(`"abc"`), &ka); err == nil {
			t.Error("expected error for invalid duration string")
		}
	})
}

func TestKeepAliveInBackendConfiguration(t *testing.T) {
	ka := KeepAlive(10 * time.Minute)
	config := BackendConfiguration{
		KeepAlive: &ka,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result BackendConfiguration
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if result.KeepAlive == nil {
		t.Fatal("expected KeepAlive to be set")
	}
	if *result.KeepAlive != ka {
		t.Errorf("expected %v, got %v", ka, *result.KeepAlive)
	}

	// Test nil KeepAlive
	config2 := BackendConfiguration{}
	data2, err := json.Marshal(config2)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result2 BackendConfiguration
	if err := json.Unmarshal(data2, &result2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result2.KeepAlive != nil {
		t.Errorf("expected nil KeepAlive, got %v", *result2.KeepAlive)
	}
}
