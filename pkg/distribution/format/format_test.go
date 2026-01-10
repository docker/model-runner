package format

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestGetFormat(t *testing.T) {
	tests := []struct {
		name      string
		format    types.Format
		wantError bool
	}{
		{"get gguf", types.FormatGGUF, false},
		{"get safetensors", types.FormatSafetensors, false},
		{"get unknown", types.Format("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Get(tt.format)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if f.Name() != tt.format {
				t.Errorf("Got format %s, want %s", f.Name(), tt.format)
			}
		})
	}
}

func TestDetectFromPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantFormat types.Format
		wantError  bool
	}{
		{"gguf file", "model.gguf", types.FormatGGUF, false},
		{"gguf uppercase", "MODEL.GGUF", types.FormatGGUF, false},
		{"gguf with path", "/path/to/model.gguf", types.FormatGGUF, false},
		{"gguf shard", "model-00001-of-00015.gguf", types.FormatGGUF, false},

		{"safetensors file", "model.safetensors", types.FormatSafetensors, false},
		{"safetensors uppercase", "MODEL.SAFETENSORS", types.FormatSafetensors, false},
		{"safetensors with path", "/path/to/model.safetensors", types.FormatSafetensors, false},
		{"safetensors shard", "model-00001-of-00003.safetensors", types.FormatSafetensors, false},

		{"unknown extension", "model.bin", types.Format(""), true},
		{"config file", "config.json", types.Format(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := DetectFromPath(tt.path)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if f.Name() != tt.wantFormat {
				t.Errorf("Got format %s, want %s", f.Name(), tt.wantFormat)
			}
		})
	}
}

func TestDetectFromPaths(t *testing.T) {
	tests := []struct {
		name       string
		paths      []string
		wantFormat types.Format
		wantError  bool
	}{
		{
			name:       "single gguf",
			paths:      []string{"model.gguf"},
			wantFormat: types.FormatGGUF,
			wantError:  false,
		},
		{
			name:       "multiple gguf",
			paths:      []string{"model-00001.gguf", "model-00002.gguf"},
			wantFormat: types.FormatGGUF,
			wantError:  false,
		},
		{
			name:       "single safetensors",
			paths:      []string{"model.safetensors"},
			wantFormat: types.FormatSafetensors,
			wantError:  false,
		},
		{
			name:       "multiple safetensors",
			paths:      []string{"model-00001-of-00002.safetensors", "model-00002-of-00002.safetensors"},
			wantFormat: types.FormatSafetensors,
			wantError:  false,
		},
		{
			name:      "mixed formats",
			paths:     []string{"model.gguf", "model.safetensors"},
			wantError: true,
		},
		{
			name:      "empty paths",
			paths:     []string{},
			wantError: true,
		},
		{
			name:      "unknown file",
			paths:     []string{"config.json"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := DetectFromPaths(tt.paths)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if f.Name() != tt.wantFormat {
				t.Errorf("Got format %s, want %s", f.Name(), tt.wantFormat)
			}
		})
	}
}

func TestGGUFFormat_Name(t *testing.T) {
	f := &GGUFFormat{}
	if f.Name() != types.FormatGGUF {
		t.Errorf("Expected %s, got %s", types.FormatGGUF, f.Name())
	}
}

func TestGGUFFormat_MediaType(t *testing.T) {
	f := &GGUFFormat{}
	if f.MediaType() != types.MediaTypeGGUF {
		t.Errorf("Expected %s, got %s", types.MediaTypeGGUF, f.MediaType())
	}
}

func TestSafetensorsFormat_Name(t *testing.T) {
	f := &SafetensorsFormat{}
	if f.Name() != types.FormatSafetensors {
		t.Errorf("Expected %s, got %s", types.FormatSafetensors, f.Name())
	}
}

func TestSafetensorsFormat_MediaType(t *testing.T) {
	f := &SafetensorsFormat{}
	if f.MediaType() != types.MediaTypeSafetensors {
		t.Errorf("Expected %s, got %s", types.MediaTypeSafetensors, f.MediaType())
	}
}

func TestNormalizeUnitString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"16.78 M", "16.78M"},
		{"256.35 MiB", "256.35MiB"},
		{"409M", "409M"},
		{"1.5 B", "1.5B"},
		{"  100 KB  ", "100KB"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeUnitString(tt.input)
			if got != tt.want {
				t.Errorf("normalizeUnitString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatParameters(t *testing.T) {
	tests := []struct {
		params int64
		want   string
	}{
		{1000, "1.00K"},
		{1000000, "1.00M"},
		{1000000000, "1.00B"},
		{7000000000, "7.00B"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatParameters(tt.params)
			if got != tt.want {
				t.Errorf("formatParameters(%d) = %q, want %q", tt.params, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{1000, "1.00kB"},
		{1000000, "1.00MB"},
		{1000000000, "1.00GB"},
		{5000000000, "5.00GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
