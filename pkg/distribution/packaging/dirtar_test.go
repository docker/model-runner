package packaging

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDirTarArchive(t *testing.T) {
	// Create a temporary directory with some test files
	tempDir, err := os.MkdirTemp("", "dirtar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test directory structure
	testDir := filepath.Join(tempDir, "test_directory")
	if err := os.MkdirAll(filepath.Join(testDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create test files
	testFiles := map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}

	for relPath, content := range testFiles {
		fullPath := filepath.Join(testDir, relPath)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file %s: %v", relPath, err)
		}
	}

	// Create tar archive
	tarPath, err := CreateDirectoryTarArchive(testDir)
	if err != nil {
		t.Fatalf("CreateDirectoryTarArchive failed: %v", err)
	}
	defer os.Remove(tarPath)

	// Verify tar archive exists
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		t.Fatal("Tar archive was not created")
	}

	// Read and verify tar contents
	file, err := os.Open(tarPath)
	if err != nil {
		t.Fatalf("Failed to open tar archive: %v", err)
	}
	defer file.Close()

	tr := tar.NewReader(file)
	foundFiles := make(map[string]bool)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read tar header: %v", err)
		}

		foundFiles[header.Name] = true

		// Verify it's within the test_directory structure
		if header.Typeflag == tar.TypeReg {
			// Read file content
			content, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("Failed to read file content: %v", err)
			}

			// Verify content matches
			expectedPath := header.Name[len("test_directory/"):]
			if expectedContent, ok := testFiles[expectedPath]; ok {
				if string(content) != expectedContent {
					t.Errorf("File %s content mismatch: got %q, want %q", expectedPath, string(content), expectedContent)
				}
			}
		}
	}

	// Verify all expected entries are present
	expectedEntries := []string{
		"test_directory",
		"test_directory/file1.txt",
		"test_directory/subdir",
		"test_directory/subdir/file2.txt",
	}

	for _, entry := range expectedEntries {
		if !foundFiles[entry] {
			t.Errorf("Expected entry %q not found in tar archive", entry)
		}
	}
}

func TestCreateDirTarArchive_NonExistentDir(t *testing.T) {
	_, err := CreateDirectoryTarArchive("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory, got nil")
	}
}

func TestCreateDirTarArchive_NotADirectory(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "not-a-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	_, err = CreateDirectoryTarArchive(tempFile.Name())
	if err == nil {
		t.Error("Expected error for file path instead of directory, got nil")
	}
}
