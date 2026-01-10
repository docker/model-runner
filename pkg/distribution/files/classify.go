// Package files provides utilities for classifying and working with model files.
// This package consolidates file classification logic used across the distribution system.
package files

import (
	"path/filepath"
	"strings"
)

// FileType represents the type of file for model packaging
type FileType int

const (
	// FileTypeUnknown is an unrecognized file type
	FileTypeUnknown FileType = iota
	// FileTypeGGUF is a GGUF model weight file
	FileTypeGGUF
	// FileTypeSafetensors is a safetensors model weight file
	FileTypeSafetensors
	// FileTypeConfig is a configuration file (json, txt, etc.)
	FileTypeConfig
	// FileTypeLicense is a license file
	FileTypeLicense
	// FileTypeChatTemplate is a Jinja chat template file
	FileTypeChatTemplate
)

// String returns a string representation of the file type
func (ft FileType) String() string {
	switch ft {
	case FileTypeGGUF:
		return "gguf"
	case FileTypeSafetensors:
		return "safetensors"
	case FileTypeConfig:
		return "config"
	case FileTypeLicense:
		return "license"
	case FileTypeChatTemplate:
		return "chat_template"
	case FileTypeUnknown:
		return "unknown"
	}
	return "unknown"
}

var (
	// ConfigExtensions defines the file extensions that should be treated as config files
	ConfigExtensions = []string{".md", ".txt", ".json", ".vocab"}

	// SpecialConfigFiles are specific filenames treated as config files
	SpecialConfigFiles = []string{"tokenizer.model"}

	// ChatTemplateExtensions defines extensions for chat template files
	ChatTemplateExtensions = []string{".jinja"}

	// LicensePatterns defines patterns for license files (case-insensitive)
	LicensePatterns = []string{"license", "licence", "copying", "notice"}
)

// Classify determines the file type based on the filename.
// It examines the file extension and name patterns to classify the file.
func Classify(path string) FileType {
	filename := filepath.Base(path)
	lower := strings.ToLower(filename)

	// Check for GGUF files first (highest priority for model files)
	if strings.HasSuffix(lower, ".gguf") {
		return FileTypeGGUF
	}

	// Check for safetensors files
	if strings.HasSuffix(lower, ".safetensors") {
		return FileTypeSafetensors
	}

	// Check for chat template files (before generic config check)
	for _, ext := range ChatTemplateExtensions {
		if strings.HasSuffix(lower, ext) {
			return FileTypeChatTemplate
		}
	}

	// Also check for files containing "chat_template" in the name
	if strings.Contains(lower, "chat_template") {
		return FileTypeChatTemplate
	}

	// Check for license files
	for _, pattern := range LicensePatterns {
		if strings.Contains(lower, pattern) {
			return FileTypeLicense
		}
	}

	// Check for config file extensions
	for _, ext := range ConfigExtensions {
		if strings.HasSuffix(lower, ext) {
			return FileTypeConfig
		}
	}

	// Check for special config files
	for _, special := range SpecialConfigFiles {
		if strings.EqualFold(lower, special) {
			return FileTypeConfig
		}
	}

	return FileTypeUnknown
}
