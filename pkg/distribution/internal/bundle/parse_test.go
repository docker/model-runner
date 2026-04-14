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

	expectedErrMsg := "no supported model weights found (neither GGUF, safetensors, nor DDUF)"
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

func TestParse_WithNestedGGUF(t *testing.T) {
	// Create a temporary directory for the test bundle.
	tempDir := t.TempDir()

	// Create model subdirectory.
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create nested directory structure.
	weightsDir := filepath.Join(modelDir, "nested", "weights")
	if err := os.MkdirAll(weightsDir, 0755); err != nil {
		t.Fatalf("Failed to create nested weights directory: %v", err)
	}

	// Create a GGUF file in the nested directory.
	nestedGGUFPath := filepath.Join(weightsDir, "model.gguf")
	if err := os.WriteFile(nestedGGUFPath, []byte("dummy nested gguf"), 0644); err != nil {
		t.Fatalf("Failed to create nested GGUF file: %v", err)
	}

	// Create a valid config.json at bundle root.
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

	// Parse the bundle and ensure GGUF discovery falls back to recursion.
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with nested GGUF, got: %v", err)
	}

	expectedPath := filepath.Join("nested", "weights", "model.gguf")
	if bundle.ggufFile != expectedPath {
		t.Errorf("Expected ggufFile to be %q, got: %s", expectedPath, bundle.ggufFile)
	}

	fullPath := bundle.GGUFPath()
	if fullPath == "" {
		t.Error("Expected GGUFPath() to return a non-empty path")
	}
	if !strings.HasSuffix(fullPath, expectedPath) {
		t.Errorf("Expected GGUFPath() to end with %q, got: %s", expectedPath, fullPath)
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

func TestParse_WithDDUF(t *testing.T) {
	// Create a temporary directory for the test bundle.
	tempDir := t.TempDir()

	// Create model subdirectory.
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create a dummy DDUF file.
	ddufPath := filepath.Join(modelDir, "model.dduf")
	if err := os.WriteFile(ddufPath, []byte("dummy dduf content"), 0644); err != nil {
		t.Fatalf("Failed to create DDUF file: %v", err)
	}

	// Create a valid config.json at bundle root.
	cfg := types.Config{
		Format: types.FormatDDUF,
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

	// Parse the bundle and ensure DDUF-only bundles are accepted.
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with DDUF file, got: %v", err)
	}

	if bundle.ddufFile != "model.dduf" {
		t.Errorf("Expected ddufFile to be %q, got: %s", "model.dduf", bundle.ddufFile)
	}

	fullPath := bundle.DDUFPath()
	if fullPath == "" {
		t.Error("Expected DDUFPath() to return a non-empty path")
	}
	if !strings.HasSuffix(fullPath, "model.dduf") {
		t.Errorf("Expected DDUFPath() to end with %q, got: %s", "model.dduf", fullPath)
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

func TestParse_WithNestedDDUF(t *testing.T) {
	// Create a temporary directory for the test bundle.
	tempDir := t.TempDir()

	// Create model subdirectory.
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create nested directory structure.
	diffusersDir := filepath.Join(modelDir, "sanitized", "diffusers")
	if err := os.MkdirAll(diffusersDir, 0755); err != nil {
		t.Fatalf("Failed to create nested diffusers directory: %v", err)
	}

	// Create a DDUF file in the nested directory.
	nestedDDUFPath := filepath.Join(diffusersDir, "model.dduf")
	if err := os.WriteFile(nestedDDUFPath, []byte("dummy nested dduf"), 0644); err != nil {
		t.Fatalf("Failed to create nested DDUF file: %v", err)
	}

	// Create a valid config.json at bundle root.
	cfg := types.Config{
		Format: types.FormatDDUF,
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

	// Parse the bundle and ensure DDUF discovery falls back to recursion.
	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Expected successful parse with nested DDUF, got: %v", err)
	}

	expectedPath := filepath.Join("sanitized", "diffusers", "model.dduf")
	if bundle.ddufFile != expectedPath {
		t.Errorf("Expected ddufFile to be %q, got: %s", expectedPath, bundle.ddufFile)
	}

	fullPath := bundle.DDUFPath()
	if fullPath == "" {
		t.Error("Expected DDUFPath() to return a non-empty path")
	}
	if !strings.HasSuffix(fullPath, expectedPath) {
		t.Errorf("Expected DDUFPath() to end with %q, got: %s", expectedPath, fullPath)
	}
}

func TestParse_WithCNCFMMProjGGUF(t *testing.T) {
	// Simulate a cached CNCF ModelPack bundle where the mmproj file has a
	// .gguf extension (e.g., "mmproj-BF16.gguf") instead of .mmproj.
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	// Create the main GGUF weight and the mmproj .gguf file
	if err := os.WriteFile(filepath.Join(modelDir, "gemma-4-E2B-it-UD-Q4_K_XL.gguf"), []byte("main model"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "mmproj-BF16.gguf"), []byte("mmproj"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid config.json
	cfg := types.Config{Format: types.FormatGGUF}
	cfgBytes, marshalErr := json.Marshal(cfg)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "config.json"), cfgBytes, 0644); err != nil {
		t.Fatal(err)
	}

	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// The main GGUF file should NOT be the mmproj
	if bundle.ggufFile == "" {
		t.Fatal("Expected ggufFile to be set")
	}
	if isMMProjFilePath(bundle.ggufFile) {
		t.Errorf("ggufFile should not be an mmproj file, got: %s", bundle.ggufFile)
	}

	// The mmproj should be detected from mmproj-BF16.gguf
	if bundle.mmprojPath == "" {
		t.Fatal("Expected mmprojPath to be set for CNCF mmproj .gguf file")
	}
	if bundle.mmprojPath != "mmproj-BF16.gguf" {
		t.Errorf("mmprojPath = %q, want %q", bundle.mmprojPath, "mmproj-BF16.gguf")
	}
	if bundle.MMPROJPath() == "" {
		t.Fatal("Expected MMPROJPath() to return non-empty path")
	}
}

func TestParse_WithTraditionalMMProj(t *testing.T) {
	// Ensure traditional .mmproj files still work
	tempDir := t.TempDir()
	modelDir := filepath.Join(tempDir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.mmproj"), []byte("mmproj"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := types.Config{Format: types.FormatGGUF}
	cfgBytes, marshalErr := json.Marshal(cfg)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "config.json"), cfgBytes, 0644); err != nil {
		t.Fatal(err)
	}

	bundle, err := Parse(tempDir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if bundle.mmprojPath != "model.mmproj" {
		t.Errorf("mmprojPath = %q, want %q", bundle.mmprojPath, "model.mmproj")
	}
	if bundle.ggufFile != "model.gguf" {
		t.Errorf("ggufFile = %q, want %q", bundle.ggufFile, "model.gguf")
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
