package commands

import (
	"testing"
)

func TestFormatCount(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "zero", input: 0, want: "0"},
		{name: "hundreds", input: 999, want: "999"},
		{name: "thousands", input: 1_000, want: "1.0K"},
		{name: "thousands with decimal", input: 45_600, want: "45.6K"},
		{name: "millions", input: 1_200_000, want: "1.2M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCount(tt.input); got != tt.want {
				t.Errorf("formatCount(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
