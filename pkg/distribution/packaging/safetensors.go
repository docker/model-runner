package packaging

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// configExtensions defines the file extensions that should be treated as config files
var configExtensions = []string{".md", ".txt", ".json", ".vocab", ".jinja"}

// PackageFromDirectory scans a directory for safetensors files and config files,
// creating a temporary tar archive of the config files.
// It returns the paths to safetensors files, path to temporary config archive (if created),
// and any error encountered.
func PackageFromDirectory(dirPath string) (safetensorsPaths []string, tempConfigArchive string, err error) {
	// Read directory contents (only top level, no subdirectories)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, "", fmt.Errorf("read directory: %w", err)
	}

	var configFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		name := entry.Name()
		fullPath := filepath.Join(dirPath, name)

		// Collect safetensors files
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".safetensors") {
			safetensorsPaths = append(safetensorsPaths, fullPath)
		}

		// Collect config files
		if isConfigFile(name) {
			configFiles = append(configFiles, fullPath)
		}
	}

	if len(safetensorsPaths) == 0 {
		return nil, "", fmt.Errorf("no safetensors files found in directory: %s", dirPath)
	}

	// Sort to ensure reproducible artifacts
	sort.Strings(safetensorsPaths)

	// Create temporary tar archive with config files if any exist
	if len(configFiles) > 0 {
		// Sort config files for reproducible tar archive
		sort.Strings(configFiles)

		tempConfigArchive, err = CreateTempConfigArchive(configFiles)
		if err != nil {
			return nil, "", fmt.Errorf("create config archive: %w", err)
		}
	}

	return safetensorsPaths, tempConfigArchive, nil
}

// CreateTempConfigArchive creates a temporary tar archive containing the specified config files.
// It returns the path to the temporary tar file and any error encountered.
// The caller is responsible for removing the temporary file when done.
func CreateTempConfigArchive(configFiles []string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "vllm-config-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Track success to determine if we should clean up the temp file
	shouldKeepTempFile := false
	defer func() {
		if !shouldKeepTempFile {
			os.Remove(tmpPath)
		}
	}()

	// Create tar writer
	tw := tar.NewWriter(tmpFile)

	// Add each config file to tar (preserving just filename, not full path)
	for _, filePath := range configFiles {
		if err := addFileToTar(tw, filePath); err != nil {
			tw.Close()
			tmpFile.Close()
			return "", err
		}
	}

	// Close tar writer first
	if err := tw.Close(); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("close tar writer: %w", err)
	}

	// Close temp file
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	shouldKeepTempFile = true
	return tmpPath, nil
}

// addFileToTar adds a single file to the tar archive with only its basename (not full path)
func addFileToTar(tw *tar.Writer, filePath string) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer file.Close()

	// Get file info for tar header
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file %s: %w", filePath, err)
	}

	// Create tar header (use only basename, not full path)
	header := &tar.Header{
		Name:    filepath.Base(filePath),
		Size:    fileInfo.Size(),
		Mode:    int64(fileInfo.Mode()),
		ModTime: fileInfo.ModTime(),
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %s: %w", filePath, err)
	}

	// Copy file contents
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("write tar content for %s: %w", filePath, err)
	}

	return nil
}

// isConfigFile checks if a file should be included as a config file based on its name.
// It checks for extensions listed in configExtensions and the special case of the tokenizer.model file.
func isConfigFile(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range configExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}

	return strings.EqualFold(name, "tokenizer.model")
}
