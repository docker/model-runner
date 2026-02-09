package packaging

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPackageFromDirectory_WithTokenizerModel(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"model.safetensors":     "safetensors content",
		"config.json":           `{"model_type": "test"}`,
		"tokenizer.model":       "tokenizer model binary content",
		"tokenizer_config.json": `{"tokenizer_class": "TestTokenizer"}`,
		"not.included":          `not included content`,
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Call PackageFromDirectory
	safetensorsPaths, tempConfigArchive, err := PackageFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("PackageFromDirectory failed: %v", err)
	}

	// Clean up temp archive
	if tempConfigArchive != "" {
		defer os.Remove(tempConfigArchive)
	}

	// Verify safetensors files were found
	if len(safetensorsPaths) != 1 {
		t.Errorf("Expected 1 safetensors file, got %d", len(safetensorsPaths))
	}

	// Verify config archive was created
	if tempConfigArchive == "" {
		t.Fatal("Expected config archive to be created")
	}

	// Verify tokenizer.model is in the archive
	archiveFiles, err := readTarArchive(tempConfigArchive)
	if err != nil {
		t.Fatalf("Failed to read tar archive: %v", err)
	}

	expectedFiles := []string{"config.json", "tokenizer.model", "tokenizer_config.json"}
	sort.Strings(expectedFiles)
	sort.Strings(archiveFiles)

	if len(archiveFiles) != len(expectedFiles) {
		t.Errorf("Expected %d files in archive, got %d", len(expectedFiles), len(archiveFiles))
	}

	for i, expected := range expectedFiles {
		if i >= len(archiveFiles) || archiveFiles[i] != expected {
			t.Errorf("Expected file %s in archive, got %v", expected, archiveFiles)
		}
	}
}

func TestPackageFromDirectory_BasicFunctionality(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"model-00001-of-00002.safetensors": "safetensors content 1",
		"model-00002-of-00002.safetensors": "safetensors content 2",
		"config.json":                      `{"model_type": "test"}`,
		"merges.txt":                       "merge1 merge2",
		"tokenizer.model":                  "tokenizer content",
		"special_tokens_map.json":          `{"unk_token": "<unk>"}`,
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Call PackageFromDirectory
	safetensorsPaths, tempConfigArchive, err := PackageFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("PackageFromDirectory failed: %v", err)
	}

	// Clean up temp archive
	if tempConfigArchive != "" {
		defer os.Remove(tempConfigArchive)
	}

	// Verify safetensors files
	if len(safetensorsPaths) != 2 {
		t.Errorf("Expected 2 safetensors files, got %d", len(safetensorsPaths))
	}

	// Verify files are sorted
	for i := 0; i < len(safetensorsPaths)-1; i++ {
		if safetensorsPaths[i] > safetensorsPaths[i+1] {
			t.Error("Safetensors paths are not sorted")
		}
	}

	// Verify config archive was created
	if tempConfigArchive == "" {
		t.Fatal("Expected config archive to be created")
	}

	// Verify archive contents
	archiveFiles, err := readTarArchive(tempConfigArchive)
	if err != nil {
		t.Fatalf("Failed to read tar archive: %v", err)
	}

	expectedConfigFiles := []string{
		"config.json",
		"merges.txt",
		"tokenizer.model",
		"special_tokens_map.json",
	}
	sort.Strings(expectedConfigFiles)
	sort.Strings(archiveFiles)

	if len(archiveFiles) != len(expectedConfigFiles) {
		t.Errorf("Expected %d config files in archive, got %d", len(expectedConfigFiles), len(archiveFiles))
	}

	for i, expected := range expectedConfigFiles {
		if i >= len(archiveFiles) || archiveFiles[i] != expected {
			t.Errorf("Expected file %s in archive at position %d, got %v", expected, i, archiveFiles)
		}
	}
}

func TestPackageFromDirectory_NoSafetensorsFiles(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create only config files (no safetensors)
	files := map[string]string{
		"config.json":     `{"model_type": "test"}`,
		"tokenizer.model": "tokenizer content",
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Call PackageFromDirectory
	_, _, err := PackageFromDirectory(tempDir)
	if err == nil {
		t.Fatal("Expected error when no safetensors files found, got nil")
	}

	expectedError := "no safetensors files found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing %q, got: %v", expectedError, err)
	}
}

func TestPackageFromDirectory_OnlySafetensorsFiles(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create only safetensors files (no config files)
	files := map[string]string{
		"model.safetensors": "safetensors content",
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Call PackageFromDirectory
	safetensorsPaths, tempConfigArchive, err := PackageFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("PackageFromDirectory failed: %v", err)
	}

	// Verify safetensors files were found
	if len(safetensorsPaths) != 1 {
		t.Errorf("Expected 1 safetensors file, got %d", len(safetensorsPaths))
	}

	// Verify no config archive was created
	if tempConfigArchive != "" {
		defer os.Remove(tempConfigArchive)
		t.Error("Expected no config archive to be created when no config files exist")
	}
}

func TestPackageFromDirectory_SkipsSubdirectories(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create test files in root
	rootFiles := map[string]string{
		"model.safetensors": "safetensors content",
		"config.json":       `{"model_type": "test"}`,
	}

	for name, content := range rootFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Create subdirectory with files that should be ignored
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	subFiles := map[string]string{
		"ignored.safetensors": "should be ignored",
		"ignored.json":        `{"ignored": true}`,
	}

	for name, content := range subFiles {
		path := filepath.Join(subDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file in subdir %s: %v", name, err)
		}
	}

	// Call PackageFromDirectory
	safetensorsPaths, tempConfigArchive, err := PackageFromDirectory(tempDir)
	if err != nil {
		t.Fatalf("PackageFromDirectory failed: %v", err)
	}

	// Clean up temp archive
	if tempConfigArchive != "" {
		defer os.Remove(tempConfigArchive)
	}

	// Verify only root-level files were processed
	if len(safetensorsPaths) != 1 {
		t.Errorf("Expected 1 safetensors file from root directory, got %d", len(safetensorsPaths))
	}

	archiveFiles, err := readTarArchive(tempConfigArchive)
	if err != nil {
		t.Fatalf("Failed to read tar archive: %v", err)
	}

	if len(archiveFiles) != 1 || archiveFiles[0] != "config.json" {
		t.Errorf("Expected only config.json from root directory, got %v", archiveFiles)
	}
}

// Helper function to read tar archive and return list of file names
func readTarArchive(archivePath string) ([]string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	tr := tar.NewReader(file)
	var files []string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		files = append(files, header.Name)
	}

	return files, nil
}
