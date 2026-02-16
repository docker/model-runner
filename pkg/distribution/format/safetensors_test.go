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
