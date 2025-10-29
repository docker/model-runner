package packaging

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// DirTarProcessor handles processing of directory tar paths for packaging
type DirTarProcessor struct {
	dirTarPaths []string
	baseDir     string
	tempFiles   []string
}

// NewDirTarProcessor creates a new processor for directory tar paths
func NewDirTarProcessor(dirTarPaths []string, baseDir string) *DirTarProcessor {
	return &DirTarProcessor{
		dirTarPaths: dirTarPaths,
		baseDir:     baseDir,
		tempFiles:   make([]string, 0),
	}
}

// Process processes all directory tar paths, validates them, and creates temporary tar archives.
// Returns a list of temporary tar file paths, cleanup function, and any error encountered.
// The caller is responsible for adding these tar files to the builder.
func (p *DirTarProcessor) Process() ([]string, func(), error) {
	var tarPaths []string

	// Get absolute paths for robust security validation
	absBase, err := filepath.Abs(p.baseDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve base directory: %w", err)
	}

	// Return cleanup function
	cleanup := func() {
		for _, tempFile := range p.tempFiles {
			os.Remove(tempFile)
		}
	}

	for _, relDirPath := range p.dirTarPaths {
		// Reject absolute paths
		if filepath.IsAbs(relDirPath) {
			return nil, cleanup, fmt.Errorf("dir-tar path must be relative: %s", relDirPath)
		}

		// Resolve the full directory path
		fullDirPath := filepath.Join(p.baseDir, relDirPath)
		fullDirPath = filepath.Clean(fullDirPath)

		absFull, err := filepath.Abs(fullDirPath)
		if err != nil {
			return nil, cleanup, fmt.Errorf("resolve full path: %w", err)
		}

		// Use filepath.Rel to compute the relative path from base to full
		// This is the canonical way to check if a path is within another
		relPathCheck, err := filepath.Rel(absBase, absFull)
		if err != nil {
			return nil, cleanup, fmt.Errorf("dir-tar path %q could not be validated: %w", relDirPath, err)
		}

		// If the relative path starts with ".." as a path component (not just as prefix),
		// it means absFull is outside absBase. We check for ".." followed by separator
		// or as the entire path to avoid false positives with directories like "..data"
		if relPathCheck == ".." || strings.HasPrefix(relPathCheck, ".."+string(os.PathSeparator)) {
			return nil, cleanup, fmt.Errorf("dir-tar path %q escapes base directory", relDirPath)
		}

		// Use Lstat (not Stat) to check if the path itself is a symlink
		// Stat would follow the symlink, but we want to detect symlinks themselves
		linfo, err := os.Lstat(fullDirPath)
		if err != nil {
			return nil, cleanup, fmt.Errorf("cannot access directory %q (resolved from %q): %w", fullDirPath, relDirPath, err)
		}

		// Reject symlinks to prevent empty tar archives (CreateDirectoryTarArchive skips symlinks)
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil, cleanup, fmt.Errorf("path %q is a symlink; symlinked directories are not supported", relDirPath)
		}

		// Verify it's a directory
		if !linfo.IsDir() {
			return nil, cleanup, fmt.Errorf("path %q is not a directory", fullDirPath)
		}

		tempTarPath, err := CreateDirectoryTarArchive(fullDirPath)
		if err != nil {
			return nil, cleanup, fmt.Errorf("create tar archive for directory %q: %w", relDirPath, err)
		}
		p.tempFiles = append(p.tempFiles, tempTarPath)
		tarPaths = append(tarPaths, tempTarPath)
	}

	return tarPaths, cleanup, nil
}
