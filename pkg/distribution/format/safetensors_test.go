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

func TestReadContextSizeConfig(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		expected  int32
		expectErr bool
	}{
		{
			name:     "max_position_embeddings",
			contents: `{"max_position_embeddings": 4096}`,
			expected: 4096,
		},
		{
			name:     "n_ctx fallback",
			contents: `{"n_ctx": 8192}`,
			expected: 8192,
		},
		{
			name:     "n_positions fallback",
			contents: `{"n_positions": 2048}`,
			expected: 2048,
		},
		{
			name:     "max_length fallback",
			contents: `{"max_length": 1024}`,
			expected: 1024,
		},
		{
			name:     "max_sequence_length fallback",
			contents: `{"max_sequence_length": 512}`,
			expected: 512,
		},
		{
			name:     "model_max_length fallback",
			contents: `{"model_max_length": 256}`,
			expected: 256,
		},
		{
			name:     "max_position_embeddings preferred over fallbacks",
			contents: `{"max_position_embeddings": 4096, "n_positions": 2048, "n_ctx": 1024}`,
			expected: 4096,
		},
		{
			name:     "n_ctx preferred over n_positions",
			contents: `{"n_ctx": 8192, "n_positions": 2048}`,
			expected: 8192,
		},
		{
			name:     "no recognized key",
			contents: `{"hidden_size": 768}`,
		},
		{
			name:     "zero value ignored",
			contents: `{"max_position_embeddings": 0}`,
		},
		{
			name:     "negative value ignored",
			contents: `{"max_position_embeddings": -1}`,
		},
		{
			name:     "value exceeding int32 ignored",
			contents: `{"max_position_embeddings": 9999999999}`,
		},
		{
			name:     "non-numeric value falls through",
			contents: `{"max_position_embeddings": "not-a-number", "n_positions": 512}`,
			expected: 512,
		},
		{
			name:      "malformed json surfaces error",
			contents:  `{not json}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(tt.contents), 0o644); err != nil {
				t.Fatalf("failed to write config.json: %v", err)
			}

			paths := []string{filepath.Join(tmpDir, "model.safetensors")}
			got, err := readContextSizeConfig(paths)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.expected == 0 {
				if got != nil {
					t.Errorf("expected nil, got %d", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %d, got nil", tt.expected)
			}
			if *got != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, *got)
			}
		})
	}
}

func TestReadContextSizeConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	paths := []string{filepath.Join(tmpDir, "model.safetensors")}
	got, err := readContextSizeConfig(paths)
	if err != nil {
		t.Fatalf("expected no error for missing config.json, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing config.json, got %d", *got)
	}
}

func TestReadContextSizeConfig_SearchesAllDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirB, "config.json"), []byte(`{"n_ctx": 4096}`), 0o644); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	paths := []string{
		filepath.Join(dirA, "shard-00001.safetensors"),
		filepath.Join(dirB, "shard-00002.safetensors"),
	}
	got, err := readContextSizeConfig(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != 4096 {
		t.Errorf("expected 4096, got %v", got)
	}
}

func TestReadContextSizeConfig_SizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("create config.json: %v", err)
	}
	if _, err := f.WriteString(`{"max_position_embeddings": 4096, "padding": "`); err != nil {
		f.Close()
		t.Fatalf("write prefix: %v", err)
	}
	chunk := make([]byte, 1024*1024)
	for i := range chunk {
		chunk[i] = 'x'
	}
	for written := 0; written <= maxFormatJSONSize; written += len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			f.Close()
			t.Fatalf("pad: %v", err)
		}
	}
	if _, err := f.WriteString(`"}`); err != nil {
		f.Close()
		t.Fatalf("write suffix: %v", err)
	}
	f.Close()

	paths := []string{filepath.Join(tmpDir, "model.safetensors")}
	got, err := readContextSizeConfig(paths)
	if err == nil {
		t.Fatalf("expected size-limit error, got nil (value: %v)", got)
	}
	if got != nil {
		t.Errorf("expected nil value on size error, got %d", *got)
	}
}
