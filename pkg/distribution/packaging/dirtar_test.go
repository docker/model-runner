package packaging

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateDirTarArchive(t *testing.T) {
	tempDir := t.TempDir()

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

func TestDirTarProcessor_ValidRelativePaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create test subdirectories
	subDir1 := filepath.Join(tempDir, "config")
	subDir2 := filepath.Join(tempDir, "templates")
	if err := os.MkdirAll(subDir1, 0755); err != nil {
		t.Fatalf("Failed to create subdir1: %v", err)
	}
	if err := os.MkdirAll(subDir2, 0755); err != nil {
		t.Fatalf("Failed to create subdir2: %v", err)
	}

	// Create test files
	if err := os.WriteFile(filepath.Join(subDir1, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir2, "template.txt"), []byte("template"), 0644); err != nil {
		t.Fatalf("Failed to write template file: %v", err)
	}

	// Test processing valid relative paths
	processor := NewDirTarProcessor([]string{"config", "templates"}, tempDir)
	tarPaths, cleanup, err := processor.Process()
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	defer cleanup()

	// Verify we got 2 tar files
	if len(tarPaths) != 2 {
		t.Errorf("Expected 2 tar files, got %d", len(tarPaths))
	}

	// Verify tar files exist
	for _, tarPath := range tarPaths {
		if _, err := os.Stat(tarPath); os.IsNotExist(err) {
			t.Errorf("Tar file does not exist: %s", tarPath)
		}
	}

	// Test cleanup
	cleanup()
	for _, tarPath := range tarPaths {
		if _, err := os.Stat(tarPath); !os.IsNotExist(err) {
			t.Errorf("Tar file was not cleaned up: %s", tarPath)
		}
	}
}

func TestDirTarProcessor_DirectoryTraversal_DoubleDot(t *testing.T) {
	tempDir := t.TempDir()

	// Test various directory traversal attempts using OS-specific path separator
	sep := string(os.PathSeparator)
	traversalAttempts := []string{
		"..",
		".." + sep,                      // "../" on Unix, "..\\" on Windows
		".." + sep + ".." + sep + "etc", // "../../etc" or "..\\..\\etc"
		".." + sep + ".." + sep + ".." + sep + "etc", // "../../../etc" or "..\\..\\..\\etc"
		"foo" + sep + ".." + sep + ".." + sep + ".." + sep + "etc",
		"subdir" + sep + ".." + sep + ".." + sep + "..",
		"." + sep + ".." + sep + ".." + sep + "etc",
	}

	for _, attempt := range traversalAttempts {
		t.Run(attempt, func(t *testing.T) {
			processor := NewDirTarProcessor([]string{attempt}, tempDir)
			_, _, err := processor.Process()
			if err == nil {
				t.Errorf("Expected error for traversal attempt %q, got nil", attempt)
			}
			if err != nil && !strings.Contains(err.Error(), "escapes base directory") {
				t.Errorf("Expected 'escapes base directory' error for %q, got: %v", attempt, err)
			}
		})
	}
}

func TestDirTarProcessor_AbsolutePathRejection(t *testing.T) {
	tempDir := t.TempDir()

	// Test absolute path rejection
	absolutePaths := []string{
		"/etc/passwd",
		"/usr/bin",
		filepath.Join(tempDir, "subdir"), // Absolute even though within tempDir
	}

	for _, absPath := range absolutePaths {
		t.Run(absPath, func(t *testing.T) {
			processor := NewDirTarProcessor([]string{absPath}, tempDir)
			_, _, err := processor.Process()
			if err == nil {
				t.Errorf("Expected error for absolute path %q, got nil", absPath)
			}
			if err != nil && !strings.Contains(err.Error(), "must be relative") {
				t.Errorf("Expected 'must be relative' error for %q, got: %v", absPath, err)
			}
		})
	}
}

func TestDirTarProcessor_NonExistentDirectory(t *testing.T) {
	processor := NewDirTarProcessor([]string{"nonexistent"}, t.TempDir())
	_, _, err := processor.Process()
	if err == nil {
		t.Error("Expected error for non-existent directory, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "cannot access directory") {
		t.Errorf("Expected 'cannot access directory' error, got: %v", err)
	}
}

func TestDirTarProcessor_FileInsteadOfDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file instead of directory
	filePath := filepath.Join(tempDir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	processor := NewDirTarProcessor([]string{"not-a-dir.txt"}, tempDir)
	_, _, err := processor.Process()
	if err == nil {
		t.Error("Expected error for file instead of directory, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("Expected 'not a directory' error, got: %v", err)
	}
}

func TestDirTarProcessor_BaseDirItself(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file in the base directory
	if err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with "." which should reference the base directory itself
	processor := NewDirTarProcessor([]string{"."}, tempDir)
	tarPaths, cleanup, err := processor.Process()
	if err != nil {
		t.Fatalf("Process failed for base directory: %v", err)
	}
	defer cleanup()

	if len(tarPaths) != 1 {
		t.Errorf("Expected 1 tar file, got %d", len(tarPaths))
	}
}

func TestDirTarProcessor_EmptyList(t *testing.T) {
	processor := NewDirTarProcessor([]string{}, t.TempDir())
	tarPaths, cleanup, err := processor.Process()
	if err != nil {
		t.Fatalf("Process failed for empty list: %v", err)
	}
	defer cleanup()

	if len(tarPaths) != 0 {
		t.Errorf("Expected 0 tar files for empty list, got %d", len(tarPaths))
	}
}

func TestDirTarProcessor_DoubleDotPrefixedDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create directories with names that start with ".." but are not traversal attempts
	doubleDotDirs := []string{
		"..data",
		"..gitkeep",
		"..metadata",
		"..2024_backup",
	}

	for _, dirName := range doubleDotDirs {
		dirPath := filepath.Join(tempDir, dirName)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create test directory %s: %v", dirName, err)
		}
		// Add a file so the tar isn't empty
		if err := os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Test processing these directories
	processor := NewDirTarProcessor(doubleDotDirs, tempDir)
	tarPaths, cleanup, err := processor.Process()
	if err != nil {
		t.Fatalf("Process failed for double-dot prefixed directories: %v", err)
	}
	defer cleanup()

	if len(tarPaths) != len(doubleDotDirs) {
		t.Errorf("Expected %d tar files, got %d", len(doubleDotDirs), len(tarPaths))
	}

	// Verify tar files exist and are not empty
	for i, tarPath := range tarPaths {
		info, err := os.Stat(tarPath)
		if err != nil {
			t.Errorf("Failed to stat test file %q: %v", tarPath, err)
		}
		if info.Size() == 0 {
			t.Errorf("Tar file is empty for %s", doubleDotDirs[i])
		}
	}
}

func TestDirTarProcessor_SymlinkedDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create a real directory
	realDir := filepath.Join(tempDir, "realdir")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("Failed to create real directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a symlink to the directory
	symlinkPath := filepath.Join(tempDir, "symlink")
	if err := os.Symlink(realDir, symlinkPath); err != nil {
		t.Skipf("Cannot create symlink (may not be supported on this system): %v", err)
	}

	// Test that the symlink is rejected
	processor := NewDirTarProcessor([]string{"symlink"}, tempDir)
	_, _, err := processor.Process()
	if err == nil {
		t.Error("Expected error for symlinked directory, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Expected error about symlink, got: %v", err)
	}
}
