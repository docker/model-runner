package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// GetRepositoryName converts a file path to a repository name
func GetRepositoryName(filePath string, baseDir string) string {
	// Convert the path to be relative to the project root
	relPath := strings.TrimPrefix(filePath, baseDir)
	// Remove leading slash if present
	relPath = strings.TrimPrefix(relPath, string(os.PathSeparator))
	// Remove file extension
	return strings.TrimSuffix(relPath, filepath.Ext(filePath))
}

// FileExists checks if a file exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}
