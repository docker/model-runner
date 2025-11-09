package utils

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple arguments",
			input:    "arg1 arg2 arg3",
			expected: []string{"arg1", "arg2", "arg3"},
		},
		{
			name:     "quoted arguments",
			input:    `arg1 "arg with spaces" arg3`,
			expected: []string{"arg1", "arg with spaces", "arg3"},
		},
		{
			name:     "single quoted arguments",
			input:    `arg1 'arg with spaces' arg3`,
			expected: []string{"arg1", "arg with spaces", "arg3"},
		},
		{
			name:     "mixed quotes",
			input:    `arg1 "double quoted" 'single quoted' arg4`,
			expected: []string{"arg1", "double quoted", "single quoted", "arg4"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "flags with values",
			input:    "--flag1 value1 --flag2 value2",
			expected: []string{"--flag1", "value1", "--flag2", "value2"},
		},
		{
			name:     "flags with quoted values",
			input:    `--flag1 "value with spaces" --flag2 value2`,
			expected: []string{"--flag1", "value with spaces", "--flag2", "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitArgs(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("SplitArgs(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
