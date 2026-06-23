package commands

import (
	"strings"
	"testing"

	"github.com/docker/model-runner/cmd/cli/search"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "zero returns empty", input: 0, want: ""},
		{name: "negative returns empty", input: -1, want: ""},
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

func TestPrettyPrintSearchResults(t *testing.T) {
	results := []search.SearchResult{
		{
			Name:        "ai/llama3.2",
			Description: "Meta Llama 3.2",
			Backend:     "llama.cpp",
			Size:        4_300_000_000,
			Downloads:   1_200_000,
			Stars:       500,
			Source:      search.DockerHubSourceName,
		},
		{
			Name:        "meta-llama/Llama-3.2-1B",
			Description: "text-generation",
			Backend:     "llama.cpp, vllm",
			Size:        0,
			Downloads:   50_000,
			Stars:       120,
			Source:      search.HuggingFaceSourceName,
		},
	}

	output := prettyPrintSearchResults(results)

	checks := []struct {
		desc string
		want string
	}{
		{"header NAME", "NAME"},
		{"header SIZE", "SIZE"},
		{"header SOURCE", "SOURCE"},
		{"docker hub model name", "ai/llama3.2"},
		{"huggingface prefix added", "hf.co/meta-llama/Llama-3.2-1B"},
		{"size formatted", "4.30GB"},
		{"unknown size empty", ""},
		{"downloads formatted", "1.2M"},
		{"source docker hub", search.DockerHubSourceName},
		{"source huggingface", search.HuggingFaceSourceName},
	}

	for _, c := range checks {
		t.Run(c.desc, func(t *testing.T) {
			if c.want != "" && !strings.Contains(output, c.want) {
				t.Errorf("output missing %q\n%s", c.want, output)
			}
		})
	}

	// HuggingFace names must not appear without the hf.co/ prefix
	if strings.Contains(output, "| meta-llama/") {
		t.Errorf("HuggingFace model name should be prefixed with hf.co/, got:\n%s", output)
	}
}
