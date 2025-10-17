package packaging

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CreateDirectoryTarArchive creates a temporary tar archive containing the specified directory
// with its structure preserved. Symlinks encountered in the directory are skipped and will not be included
// in the archive. It returns the path to the temporary tar file and any error encountered.
// The caller is responsible for removing the temporary file when done.
func CreateDirectoryTarArchive(dirPath string) (string, error) {
	// Verify directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", dirPath)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "dir-tar-*.tar")
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

	// Walk the directory tree
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return fmt.Errorf("nil FileInfo for path: %s", path)
		}
		// Skip symlinks - they're not needed for model distribution and are
		// skipped during extraction for security reasons
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header for %s: %w", path, err)
		}

		// Compute relative path from the parent of dirPath
		relPath, err := filepath.Rel(filepath.Dir(dirPath), path)
		if err != nil {
			return fmt.Errorf("compute relative path: %w", err)
		}

		// Use forward slashes for tar archive paths
		header.Name = filepath.ToSlash(relPath)

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open file %s: %w", path, err)
			}

			// Copy file contents
			if _, err := io.Copy(tw, file); err != nil {
				file.Close()
				return fmt.Errorf("write tar content for %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		tw.Close()
		tmpFile.Close()
		return "", fmt.Errorf("walk directory: %w", err)
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
