package format

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSafetensorsHeader_TruncatedFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "truncated.safetensors")

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	headerLen := uint64(1000)
	if writeErr := binary.Write(file, binary.LittleEndian, headerLen); writeErr != nil {
		file.Close()
		t.Fatalf("failed to write header length: %v", writeErr)
	}

	truncatedJSON := make([]byte, 500)
	copy(truncatedJSON, []byte(`{"incomplete": "json`))
	if _, writeTruncErr := file.Write(truncatedJSON); writeTruncErr != nil {
		file.Close()
		t.Fatalf("failed to write truncated data: %v", writeTruncErr)
	}
	file.Close()

	if _, err := parseSafetensorsHeader(filePath); err == nil {
		t.Fatal("expected error for truncated safetensors header, got nil")
	}
}

func TestReadContextSizeFromConfigJSON(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		expected *int32
	}{
		{
			name:     "max_position_embeddings",
			contents: `{"max_position_embeddings": 4096}`,
			expected: int32Ptr(4096),
		},
		{
			name:     "n_ctx fallback",
			contents: `{"n_ctx": 8192}`,
			expected: int32Ptr(8192),
		},
		{
			name:     "n_positions fallback",
			contents: `{"n_positions": 2048}`,
			expected: int32Ptr(2048),
		},
		{
			name:     "max_length fallback",
			contents: `{"max_length": 1024}`,
			expected: int32Ptr(1024),
		},
		{
			name:     "max_sequence_length fallback",
			contents: `{"max_sequence_length": 512}`,
			expected: int32Ptr(512),
		},
		{
			name:     "model_max_length fallback",
			contents: `{"model_max_length": 256}`,
			expected: int32Ptr(256),
		},
		{
			name:     "max_position_embeddings preferred over fallbacks",
			contents: `{"max_position_embeddings": 4096, "n_positions": 2048, "n_ctx": 1024}`,
			expected: int32Ptr(4096),
		},
		{
			name:     "n_ctx preferred over n_positions",
			contents: `{"n_ctx": 8192, "n_positions": 2048}`,
			expected: int32Ptr(8192),
		},
		{
			name:     "no recognized key",
			contents: `{"hidden_size": 768}`,
			expected: nil,
		},
		{
			name:     "zero value ignored",
			contents: `{"max_position_embeddings": 0}`,
			expected: nil,
		},
		{
			name:     "negative value ignored",
			contents: `{"max_position_embeddings": -1}`,
			expected: nil,
		},
		{
			name:     "value exceeding int32 ignored",
			contents: `{"max_position_embeddings": 9999999999}`,
			expected: nil,
		},
		{
			name:     "non-numeric value falls through",
			contents: `{"max_position_embeddings": "not-a-number", "n_positions": 512}`,
			expected: int32Ptr(512),
		},
		{
			name:     "malformed json",
			contents: `{not json}`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(tt.contents), 0o644); err != nil {
				t.Fatalf("failed to write config.json: %v", err)
			}

			got := readContextSizeFromConfigJSON(tmpDir)
			if (got == nil) != (tt.expected == nil) {
				t.Fatalf("expected nil=%v, got nil=%v (got value: %v)", tt.expected == nil, got == nil, got)
			}
			if got != nil && *got != *tt.expected {
				t.Errorf("expected %d, got %d", *tt.expected, *got)
			}
		})
	}
}

func TestReadContextSizeFromConfigJSON_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	if got := readContextSizeFromConfigJSON(tmpDir); got != nil {
		t.Errorf("expected nil for missing config.json, got %d", *got)
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
