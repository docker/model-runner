package commands

import (
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "zero returns n/a", input: 0, want: "n/a"},
		{name: "negative returns n/a", input: -1, want: "n/a"},
		{name: "bytes", input: 500, want: "500.00B"},
		{name: "kilobytes", input: 1500, want: "1.50kB"},
		{name: "megabytes", input: 2_500_000, want: "2.50MB"},
		{name: "gigabytes", input: 4_300_000_000, want: "4.30GB"},
		{name: "terabytes", input: 1_200_000_000_000, want: "1.20TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSize(tt.input); got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

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
