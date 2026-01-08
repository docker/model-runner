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

// Classify determines the file type based on filename.
// It examines the file extension and name patterns to classify the file.
func Classify(filename string) FileType {
	lower := strings.ToLower(filename)
	baseName := filepath.Base(lower)

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
		if strings.Contains(baseName, pattern) {
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
		if strings.EqualFold(filename, special) {
			return FileTypeConfig
		}
	}

	return FileTypeUnknown
}

// IsConfigFile checks if a file should be included as a config file based on its name.
// This is a convenience function that checks for config file extensions and special names.
func IsConfigFile(name string) bool {
	ft := Classify(name)
	return ft == FileTypeConfig || ft == FileTypeChatTemplate
}

// IsWeightFile checks if a file is a model weight file (GGUF or Safetensors).
func IsWeightFile(name string) bool {
	ft := Classify(name)
	return ft == FileTypeGGUF || ft == FileTypeSafetensors
}

// IsGGUF checks if a file is a GGUF weight file.
func IsGGUF(name string) bool {
	return Classify(name) == FileTypeGGUF
}

// IsSafetensors checks if a file is a Safetensors weight file.
func IsSafetensors(name string) bool {
	return Classify(name) == FileTypeSafetensors
}

// IsChatTemplate checks if a file is a chat template file.
func IsChatTemplate(name string) bool {
	ft := Classify(name)
	return ft == FileTypeChatTemplate
}

// FilterByType filters a list of filenames by the specified file type.
func FilterByType(filenames []string, fileType FileType) []string {
	var result []string
	for _, name := range filenames {
		if Classify(filepath.Base(name)) == fileType {
			result = append(result, name)
		}
	}
	return result
}

// SplitByType categorizes a list of filenames into separate slices by type.
func SplitByType(filenames []string) (weights, configs, templates, licenses, unknown []string) {
	for _, name := range filenames {
		switch Classify(filepath.Base(name)) {
		case FileTypeGGUF, FileTypeSafetensors:
			weights = append(weights, name)
		case FileTypeConfig:
			configs = append(configs, name)
		case FileTypeChatTemplate:
			templates = append(templates, name)
		case FileTypeLicense:
			licenses = append(licenses, name)
		case FileTypeUnknown:
			unknown = append(unknown, name)
		}
	}
	return
}
