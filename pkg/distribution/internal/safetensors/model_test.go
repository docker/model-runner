package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestNewModel_WithMetadata(t *testing.T) {
	// Create a test safetensors file with metadata
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.safetensors")

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

	// Convert header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("failed to marshal header: %v", err)
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Write header length
	headerLen := uint64(len(headerJSON))
	if err := binary.Write(file, binary.LittleEndian, headerLen); err != nil {
		file.Close()
		t.Fatalf("failed to write header length: %v", err)
	}

	// Write header JSON
	if _, err := file.Write(headerJSON); err != nil {
		file.Close()
		t.Fatalf("failed to write header: %v", err)
	}

	// Write dummy tensor data
	dummyData := make([]byte, 33562624)
	if _, err := file.Write(dummyData); err != nil {
		file.Close()
		t.Fatalf("failed to write dummy data: %v", err)
	}
	file.Close()

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
	expectedParams := "16.78 M"
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

func TestNewModel_NoMetadata(t *testing.T) {
	// Create a test safetensors file without metadata section
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.safetensors")

	header := map[string]interface{}{
		"weight": map[string]interface{}{
			"dtype":        "F32",
			"shape":        []interface{}{float64(100), float64(200)},
			"data_offsets": []interface{}{float64(0), float64(80000)},
		},
	}

	// Convert header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("failed to marshal header: %v", err)
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Write header length
	headerLen := uint64(len(headerJSON))
	if err := binary.Write(file, binary.LittleEndian, headerLen); err != nil {
		file.Close()
		t.Fatalf("failed to write header length: %v", err)
	}

	// Write header JSON
	if _, err := file.Write(headerJSON); err != nil {
		file.Close()
		t.Fatalf("failed to write header: %v", err)
	}

	// Write dummy tensor data
	dummyData := make([]byte, 80000)
	if _, err := file.Write(dummyData); err != nil {
		file.Close()
		t.Fatalf("failed to write dummy data: %v", err)
	}
	file.Close()

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
	expectedParams := "20.00 K"
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
