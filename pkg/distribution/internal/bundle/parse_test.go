package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestParse_NoModelWeights(t *testing.T) {
	// Create a temporary directory for the test bundle
	tempDir := t.TempDir()

	// Create model subdirectory
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create a valid config.json at bundle root
	cfg := types.Config{
		Format: types.FormatGGUF,
	}
	configPath := filepath.Join(tempDir, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode config: %v", err)
	}
	f.Close()

	// Try to parse the bundle - should fail because no model weights are present
	_, err = Parse(tempDir)
	if err == nil {
		t.Fatal("Expected error when parsing bundle without model weights, got nil")
	}

	expectedErrMsg := "no supported model weights found (neither GGUF nor safetensors)"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedErrMsg, err)
	}
}

func TestParse_WithGGUF(t *testing.T) {
	// Create a temporary directory for the test bundle
	tempDir := t.TempDir()

	// Create model subdirectory
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create a dummy GGUF file
	ggufPath := filepath.Join(modelDir, "model.gguf")
	if err := os.WriteFile(ggufPath, []byte("dummy gguf content"), 0644); err != nil {
		t.Fatalf("Failed to create GGUF file: %v", err)
	}

	// Create a valid config.json at bundle root
	cfg := types.Config{
		Format: types.FormatGGUF,
	}
	configPath := filepath.Join(tempDir, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode config: %v", err)
	}
	f.Close()

	// Parse the bundle - should succeed
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with GGUF file, got error: %v", err)
	}

	if bundle.ggufFile != "model.gguf" {
		t.Errorf("Expected ggufFile to be 'model.gguf', got: %s", bundle.ggufFile)
	}

	if bundle.safetensorsFile != "" {
		t.Errorf("Expected safetensorsFile to be empty, got: %s", bundle.safetensorsFile)
	}
}

func TestParse_WithSafetensors(t *testing.T) {
	// Create a temporary directory for the test bundle
	tempDir := t.TempDir()

	// Create model subdirectory
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create a dummy safetensors file
	safetensorsPath := filepath.Join(modelDir, "model.safetensors")
	if err := os.WriteFile(safetensorsPath, []byte("dummy safetensors content"), 0644); err != nil {
		t.Fatalf("Failed to create safetensors file: %v", err)
	}

	// Create a valid config.json at bundle root
	cfg := types.Config{
		Format: types.FormatSafetensors,
	}
	configPath := filepath.Join(tempDir, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode config: %v", err)
	}
	f.Close()

	// Parse the bundle - should succeed
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with safetensors file, got error: %v", err)
	}

	if bundle.safetensorsFile != "model.safetensors" {
		t.Errorf("Expected safetensorsFile to be 'model.safetensors', got: %s", bundle.safetensorsFile)
	}

	if bundle.ggufFile != "" {
		t.Errorf("Expected ggufFile to be empty, got: %s", bundle.ggufFile)
	}
}

func TestParse_WithNestedSafetensors(t *testing.T) {
	// Create a temporary directory for the test bundle
	tempDir := t.TempDir()

	// Create model subdirectory
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create nested directory structure (V0.2 layout)
	textEncoderDir := filepath.Join(modelDir, "text_encoder")
	if err := os.MkdirAll(textEncoderDir, 0755); err != nil {
		t.Fatalf("Failed to create text_encoder directory: %v", err)
	}

	// Create a safetensors file in the nested directory (no safetensors at top level)
	nestedSafetensorsPath := filepath.Join(textEncoderDir, "model.safetensors")
	if err := os.WriteFile(nestedSafetensorsPath, []byte("dummy nested safetensors content"), 0644); err != nil {
		t.Fatalf("Failed to create nested safetensors file: %v", err)
	}

	// Create a valid config.json at bundle root
	cfg := types.Config{
		Format: types.FormatSafetensors,
	}
	configPath := filepath.Join(tempDir, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode config: %v", err)
	}
	f.Close()

	// Parse the bundle - should succeed by finding safetensors recursively
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with nested safetensors, got error: %v", err)
	}

	// The safetensorsFile should include the relative path from modelDir
	expectedPath := filepath.Join("text_encoder", "model.safetensors")
	if bundle.safetensorsFile != expectedPath {
		t.Errorf("Expected safetensorsFile to be %q, got: %s", expectedPath, bundle.safetensorsFile)
	}

	// Verify SafetensorsPath() returns the full path
	fullPath := bundle.SafetensorsPath()
	if fullPath == "" {
		t.Error("Expected SafetensorsPath() to return a non-empty path")
	}
	if !strings.HasSuffix(fullPath, expectedPath) {
		t.Errorf("Expected SafetensorsPath() to end with %q, got: %s", expectedPath, fullPath)
	}
}

func TestParse_WithBothFormats(t *testing.T) {
	// Create a temporary directory for the test bundle
	tempDir := t.TempDir()

	// Create model subdirectory
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create both GGUF and safetensors files
	ggufPath := filepath.Join(modelDir, "model.gguf")
	if err := os.WriteFile(ggufPath, []byte("dummy gguf content"), 0644); err != nil {
		t.Fatalf("Failed to create GGUF file: %v", err)
	}

	safetensorsPath := filepath.Join(modelDir, "model.safetensors")
	if err := os.WriteFile(safetensorsPath, []byte("dummy safetensors content"), 0644); err != nil {
		t.Fatalf("Failed to create safetensors file: %v", err)
	}

	// Create a valid config.json at bundle root
	cfg := types.Config{
		Format: types.FormatGGUF,
	}
	configPath := filepath.Join(tempDir, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode config: %v", err)
	}
	f.Close()

	// Parse the bundle - should succeed with both files present
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with both formats, got error: %v", err)
	}

	if bundle.ggufFile != "model.gguf" {
		t.Errorf("Expected ggufFile to be 'model.gguf', got: %s", bundle.ggufFile)
	}

	if bundle.safetensorsFile != "model.safetensors" {
		t.Errorf("Expected safetensorsFile to be 'model.safetensors', got: %s", bundle.safetensorsFile)
	}
}
