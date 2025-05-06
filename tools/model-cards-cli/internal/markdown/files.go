package markdown

import (
	"fmt"
	"path/filepath"
)

// FindMarkdownFiles finds all markdown files in the specified directory
func FindMarkdownFiles(directory string) ([]string, error) {
	// Use filepath.Glob to find all markdown files
	pattern := filepath.Join(directory, "*.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("error finding markdown files: %v", err)
	}

	return files, nil
}
