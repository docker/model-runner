package packaging

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/files"
)

// ModelFileInfo contains information about a model file including its relative path
type ModelFileInfo struct {
	// AbsPath is the absolute path to the file
	AbsPath string
	// RelPath is the path relative to the model root directory (uses forward slashes)
	RelPath string
	// Size is the file size in bytes
	Size int64
}

// PackageResult contains the result of packaging a model directory
type PackageResult struct {
	// WeightFiles contains info about large weight files (safetensors, GGUF, DDUF)
	// These should be packaged as separate OCI layers
	WeightFiles []ModelFileInfo
	// ConfigTarPath is the path to a temporary tar archive containing config files
	// The caller is responsible for removing this file
	ConfigTarPath string
	// Format is the detected model format based on weight files
	Format string
}

// PackageFromDirectory scans a directory for safetensors files and config files,
// creating a temporary tar archive of the config files.
// It returns the paths to safetensors files, path to temporary config archive (if created),
// and any error encountered.
// DEPRECATED: Use PackageFromDirectoryRecursive for nested directory support.
func PackageFromDirectory(dirPath string) (safetensorsPaths []string, tempConfigArchive string, err error) {
	result, err := PackageFromDirectoryRecursive(dirPath)
	if err != nil {
		return nil, "", err
	}

	// Convert to old format for backward compatibility
	for _, wf := range result.WeightFiles {
		safetensorsPaths = append(safetensorsPaths, wf.AbsPath)
	}

	return safetensorsPaths, result.ConfigTarPath, nil
}

// PackageFromDirectoryRecursive scans a directory recursively for model files,
// separating large weight files from small config files.
// Weight files are returned with their relative paths preserved.
// Config files are packaged into a tar archive with directory structure preserved.
func PackageFromDirectoryRecursive(dirPath string) (*PackageResult, error) {
	// Verify directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	var weightFiles []ModelFileInfo
	var configFiles []ModelFileInfo
	var detectedFormat string

	// Walk the directory tree recursively
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == dirPath {
			return nil
		}

		// Skip hidden files and directories (starting with .)
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories themselves (but continue walking into them)
		if info.IsDir() {
			return nil
		}

		// Skip symlinks for security
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Calculate relative path from the model root
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("compute relative path: %w", err)
		}

		// Convert to forward slashes for cross-platform compatibility
		relPath = filepath.ToSlash(relPath)

		fileInfo := ModelFileInfo{
			AbsPath: path,
			RelPath: relPath,
			Size:    info.Size(),
		}

		// Classify the file
		fileType := files.Classify(path)

		switch fileType {
		case files.FileTypeSafetensors:
			weightFiles = append(weightFiles, fileInfo)
			if detectedFormat == "" {
				detectedFormat = "safetensors"
			}
		case files.FileTypeGGUF:
			weightFiles = append(weightFiles, fileInfo)
			if detectedFormat == "" {
				detectedFormat = "gguf"
			}
		case files.FileTypeDDUF:
			weightFiles = append(weightFiles, fileInfo)
			if detectedFormat == "" {
				detectedFormat = "dduf"
			}
		case files.FileTypeConfig, files.FileTypeChatTemplate, files.FileTypeLicense:
			configFiles = append(configFiles, fileInfo)
		case files.FileTypeUnknown:
			// Include unknown files in config archive if they're small
			// This catches things like README without extension, etc.
			if info.Size() < 5*1024*1024 { // 5MB threshold
				configFiles = append(configFiles, fileInfo)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	if len(weightFiles) == 0 {
		return nil, fmt.Errorf("no weight files (safetensors, GGUF, or DDUF) found in directory: %s", dirPath)
	}

	// Sort for reproducibility
	sort.Slice(weightFiles, func(i, j int) bool {
		return weightFiles[i].RelPath < weightFiles[j].RelPath
	})
	sort.Slice(configFiles, func(i, j int) bool {
		return configFiles[i].RelPath < configFiles[j].RelPath
	})

	result := &PackageResult{
		WeightFiles: weightFiles,
		Format:      detectedFormat,
	}

	// Create config archive if there are config files
	if len(configFiles) > 0 {
		configTarPath, err := CreateConfigArchiveWithRelativePaths(configFiles)
		if err != nil {
			return nil, fmt.Errorf("create config archive: %w", err)
		}
		result.ConfigTarPath = configTarPath
	}

	return result, nil
}

// CreateConfigArchiveWithRelativePaths creates a tar archive containing files with their
// relative paths preserved. This allows the directory structure to be reconstructed
// when unpacking.
func CreateConfigArchiveWithRelativePaths(configFiles []ModelFileInfo) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "config-archive-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Track success to determine if we should clean up
	shouldKeepTempFile := false
	defer func() {
		if !shouldKeepTempFile {
			os.Remove(tmpPath)
		}
	}()

	// Create tar writer
	tw := tar.NewWriter(tmpFile)

	// Add each config file to tar with its relative path
	for _, cf := range configFiles {
		if err := addFileToTarWithRelativePath(tw, cf.AbsPath, cf.RelPath); err != nil {
			tw.Close()
			tmpFile.Close()
			return "", fmt.Errorf("add file %s to tar: %w", cf.RelPath, err)
		}
	}

	// Close tar writer
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

// addFileToTarWithRelativePath adds a file to the tar archive using the specified relative path
func addFileToTarWithRelativePath(tw *tar.Writer, absPath, relPath string) error {
	// Open the file
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("open file %s: %w", absPath, err)
	}
	defer file.Close()

	// Get file info for tar header
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file %s: %w", absPath, err)
	}

	// Create tar header with relative path (using forward slashes)
	header := &tar.Header{
		Name:    relPath, // Use the relative path to preserve directory structure
		Size:    fileInfo.Size(),
		Mode:    int64(fileInfo.Mode()),
		ModTime: fileInfo.ModTime(),
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %s: %w", relPath, err)
	}

	// Copy file contents
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("write tar content for %s: %w", relPath, err)
	}

	return nil
}

// CreateConfigArchiveInDir creates a tar archive containing the specified config files in the given directory.
// If dir is empty, the system temp directory is used.
// It returns the path to the tar file and any error encountered.
// The caller is responsible for removing the file when done.
func CreateConfigArchiveInDir(configFiles []string, dir string) (string, error) {
	// Create temp file in specified directory
	tmpFile, err := os.CreateTemp(dir, "vllm-config-*.tar")
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
