package builder_test

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/types"
)

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

func TestSafetensorsModel_WithMetadata(t *testing.T) {
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

	b, err := builder.FromPath(filePath)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	model := b.Model()

	config, err := model.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	if config.GetFormat() != types.FormatSafetensors {
		t.Errorf("Config.Format = %v, want %v", config.GetFormat(), types.FormatSafetensors)
	}

	if config.GetArchitecture() != "LlamaForCausalLM" {
		t.Errorf("Config.Architecture = %v, want %v", config.GetArchitecture(), "LlamaForCausalLM")
	}

	expectedParams := "16.78M"
	if config.GetParameters() != expectedParams {
		t.Errorf("Config.Parameters = %v, want %v", config.GetParameters(), expectedParams)
	}

	if config.GetQuantization() != "F16" {
		t.Errorf("Config.Quantization = %v, want %v", config.GetQuantization(), "F16")
	}

	if config.GetSize() == "" {
		t.Error("Config.Size is empty")
	}

	dockerConfig, ok := config.(*types.Config)
	if !ok {
		t.Fatal("Expected *types.Config for safetensors model")
	}

	if dockerConfig.Safetensors == nil {
		t.Fatal("Config.Safetensors is nil")
	}

	if dockerConfig.Safetensors["architecture"] != "LlamaForCausalLM" {
		t.Errorf("Config.Safetensors[architecture] = %v, want %v", dockerConfig.Safetensors["architecture"], "LlamaForCausalLM")
	}

	if dockerConfig.Safetensors["tensor_count"] != "2" {
		t.Errorf("Config.Safetensors[tensor_count] = %v, want %v", dockerConfig.Safetensors["tensor_count"], "2")
	}

	manifest, err := model.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}

	if len(manifest.Layers) != 1 {
		t.Fatalf("Expected 1 layer, got %d", len(manifest.Layers))
	}

	layer := manifest.Layers[0]
	if layer.Annotations == nil {
		t.Fatal("Expected annotations to be present")
	}

	if _, ok := layer.Annotations[types.AnnotationFilePath]; !ok {
		t.Errorf("Expected annotation %s to be present", types.AnnotationFilePath)
	}

	if _, ok := layer.Annotations[types.AnnotationFileMetadata]; !ok {
		t.Errorf("Expected annotation %s to be present", types.AnnotationFileMetadata)
	}

	if val, ok := layer.Annotations[types.AnnotationMediaTypeUntested]; !ok {
		t.Errorf("Expected annotation %s to be present", types.AnnotationMediaTypeUntested)
	} else if val != "false" {
		t.Errorf("Expected annotation %s to be 'false', got '%s'", types.AnnotationMediaTypeUntested, val)
	}

	metadataJSON := layer.Annotations[types.AnnotationFileMetadata]
	var metadata types.FileMetadata
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("Failed to unmarshal file metadata: %v", err)
	}

	if metadata.Name != "test.safetensors" {
		t.Errorf("Expected file name 'test.safetensors', got '%s'", metadata.Name)
	}
	if metadata.Size == 0 {
		t.Error("Expected file size to be non-zero")
	}
}
