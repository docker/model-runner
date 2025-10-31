package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
)

// createTestSafetensorsFile is a helper function that creates a test safetensors file
// with the specified header and data size.
func createTestSafetensorsFile(t *testing.T, dir string, name string, header map[string]interface{}, dataSize int) string {
	t.Helper()
	filePath := filepath.Join(dir, name)

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("failed to marshal header: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	defer file.Close()

	headerLen := uint64(len(headerJSON))
	if err := binary.Write(file, binary.LittleEndian, headerLen); err != nil {
		t.Fatalf("failed to write header length: %v", err)
	}

	if _, err := file.Write(headerJSON); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	if dataSize > 0 {
		dummyData := make([]byte, dataSize)
		if _, err := file.Write(dummyData); err != nil {
			t.Fatalf("failed to write dummy data: %v", err)
		}
	}

	return filePath
}

func TestNewModel_WithMetadata(t *testing.T) {
	// Create a test safetensors file with metadata
	tmpDir := t.TempDir()

	header := map[string]interface{}{
		"__metadata__": map[string]interface{}{
			"architecture": "LlamaForCausalLM",
			"version":      "1.0",
		},
		"model.layers.0.weight": map[string]interface{}{
			"dtype":        "F16",
			"shape":        []interface{}{float64(4096), float64(4096)},
			"data_offsets": []interface{}{float64(0), float64(33554432)},
		},
		"model.layers.0.bias": map[string]interface{}{
			"dtype":        "F16",
			"shape":        []interface{}{float64(4096)},
			"data_offsets": []interface{}{float64(33554432), float64(33562624)},
		},
	}

	filePath := createTestSafetensorsFile(t, tmpDir, "test.safetensors", header, 33562624)

	// Create model
	model, err := NewModel([]string{filePath})
	if err != nil {
		t.Fatalf("NewModel() error = %v", err)
	}

	// Get config
	config, err := model.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	// Verify format
	if config.Format != types.FormatSafetensors {
		t.Errorf("Config.Format = %v, want %v", config.Format, types.FormatSafetensors)
	}

	// Verify architecture
	if config.Architecture != "LlamaForCausalLM" {
		t.Errorf("Config.Architecture = %v, want %v", config.Architecture, "LlamaForCausalLM")
	}

	// Verify parameters (4096*4096 + 4096 = 16781312)
	expectedParams := "16.78M"
	if config.Parameters != expectedParams {
		t.Errorf("Config.Parameters = %v, want %v", config.Parameters, expectedParams)
	}

	// Verify quantization
	if config.Quantization != "F16" {
		t.Errorf("Config.Quantization = %v, want %v", config.Quantization, "F16")
	}

	// Verify size is calculated
	if config.Size == "" {
		t.Error("Config.Size is empty")
	}

	// Verify safetensors metadata map
	if config.Safetensors == nil {
		t.Fatal("Config.Safetensors is nil")
	}

	if config.Safetensors["architecture"] != "LlamaForCausalLM" {
		t.Errorf("Config.Safetensors[architecture] = %v, want %v", config.Safetensors["architecture"], "LlamaForCausalLM")
	}

	if config.Safetensors["tensor_count"] != "2" {
		t.Errorf("Config.Safetensors[tensor_count] = %v, want %v", config.Safetensors["tensor_count"], "2")
	}
}

func TestParseHeader_TruncatedFile(t *testing.T) {
	// Create a test file with incomplete header
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "truncated.safetensors")

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Write header length claiming 1000 bytes
	headerLen := uint64(1000)
	if err := binary.Write(file, binary.LittleEndian, headerLen); err != nil {
		file.Close()
		t.Fatalf("failed to write header length: %v", err)
	}

	// But only write 500 bytes (truncated)
	truncatedJSON := make([]byte, 500)
	copy(truncatedJSON, []byte(`{"incomplete": "json`))
	if _, err := file.Write(truncatedJSON); err != nil {
		file.Close()
		t.Fatalf("failed to write truncated data: %v", err)
	}
	file.Close()

	// Attempt to parse - should fail gracefully
	_, err = ParseSafetensorsHeader(filePath)
	if err == nil {
		t.Fatal("ParseSafetensorsHeader() expected error for truncated file, got nil")
	}
}

func TestParseHeader_InvalidJSON(t *testing.T) {
	// Create a test file with invalid JSON
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.safetensors")

	// Create malformed JSON
	invalidJSON := []byte(`{"missing": "closing brace", "broken": [1, 2, }`)

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Write header length
	headerLen := uint64(len(invalidJSON))
	if err := binary.Write(file, binary.LittleEndian, headerLen); err != nil {
		file.Close()
		t.Fatalf("failed to write header length: %v", err)
	}

	// Write invalid JSON
	if _, err := file.Write(invalidJSON); err != nil {
		file.Close()
		t.Fatalf("failed to write invalid JSON: %v", err)
	}
	file.Close()

	// Attempt to parse - should fail gracefully
	_, err = ParseSafetensorsHeader(filePath)
	if err == nil {
		t.Fatal("ParseSafetensorsHeader() expected error for invalid JSON, got nil")
	}
}

func TestNewModel_NoMetadata(t *testing.T) {
	// Create a test safetensors file without metadata section
	tmpDir := t.TempDir()

	header := map[string]interface{}{
		"weight": map[string]interface{}{
			"dtype":        "F32",
			"shape":        []interface{}{float64(100), float64(200)},
			"data_offsets": []interface{}{float64(0), float64(80000)},
		},
	}

	filePath := createTestSafetensorsFile(t, tmpDir, "test.safetensors", header, 80000)

	// Create model
	model, err := NewModel([]string{filePath})
	if err != nil {
		t.Fatalf("NewModel() error = %v", err)
	}

	// Get config
	config, err := model.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	// Verify format
	if config.Format != types.FormatSafetensors {
		t.Errorf("Config.Format = %v, want %v", config.Format, types.FormatSafetensors)
	}

	// Verify parameters (100*200 = 20000)
	expectedParams := "20.00K"
	if config.Parameters != expectedParams {
		t.Errorf("Config.Parameters = %v, want %v", config.Parameters, expectedParams)
	}

	// Verify quantization
	if config.Quantization != "F32" {
		t.Errorf("Config.Quantization = %v, want %v", config.Quantization, "F32")
	}

	// Architecture should be empty when no metadata
	if config.Architecture != "" {
		t.Errorf("Config.Architecture = %v, want empty string", config.Architecture)
	}

	// Verify safetensors metadata map exists with tensor count
	if config.Safetensors == nil {
		t.Fatal("Config.Safetensors is nil")
	}

	if config.Safetensors["tensor_count"] != "1" {
		t.Errorf("Config.Safetensors[tensor_count] = %v, want %v", config.Safetensors["tensor_count"], "1")
	}
}
