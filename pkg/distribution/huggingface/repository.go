package huggingface

import (
	"path"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/packaging"
)

// RepoFile represents a file in a HuggingFace repository
type RepoFile struct {
	Type string   `json:"type"` // "file" or "directory"
	Path string   `json:"path"` // Relative path in repo
	Size int64    `json:"size"` // File size in bytes (0 for directories)
	OID  string   `json:"oid"`  // Git blob ID
	LFS  *LFSInfo `json:"lfs"`  // Present if LFS file
}

// LFSInfo contains LFS-specific file information
type LFSInfo struct {
	OID         string `json:"oid"`          // LFS object ID (sha256)
	Size        int64  `json:"size"`         // Actual file size
	PointerSize int64  `json:"pointer_size"` // Size of pointer file
}

// ActualSize returns the actual file size, accounting for LFS
func (f *RepoFile) ActualSize() int64 {
	if f.LFS != nil {
		return f.LFS.Size
	}
	return f.Size
}

// Filename returns the base filename without directory path
func (f *RepoFile) Filename() string {
	return path.Base(f.Path)
}

// ModelType represents the type of model (LLM vs diffusers)
type ModelType int

const (
	// ModelTypeLLM is a standard LLM model with safetensors at root
	ModelTypeLLM ModelType = iota
	// ModelTypeDiffusers is a diffusers model with model_index.json
	ModelTypeDiffusers
)

// fileType represents the type of file for model packaging
type fileType int

const (
	// fileTypeUnknown is an unrecognized file type
	fileTypeUnknown fileType = iota
	// fileTypeSafetensors is a safetensors model weight file
	fileTypeSafetensors
	// fileTypeConfig is a configuration file (json, txt, etc.)
	fileTypeConfig
	// fileTypeDiffusersIndex is the model_index.json file for diffusers models
	fileTypeDiffusersIndex
)

// classifyFile determines the file type based on filename
func classifyFile(filename string) fileType {
	lower := strings.ToLower(filename)

	// Check for diffusers model_index.json
	if lower == "model_index.json" {
		return fileTypeDiffusersIndex
	}

	// Check for safetensors files
	if strings.HasSuffix(lower, ".safetensors") {
		return fileTypeSafetensors
	}

	// Check for config file extensions
	for _, ext := range packaging.ConfigExtensions {
		if strings.HasSuffix(lower, ext) {
			return fileTypeConfig
		}
	}

	// Check for special config files
	for _, special := range packaging.SpecialConfigFiles {
		if strings.EqualFold(filename, special) {
			return fileTypeConfig
		}
	}

	return fileTypeUnknown
}

// FilterModelFiles filters repository files to only include files needed for model-runner
// Returns safetensors files and config files separately
func FilterModelFiles(files []RepoFile) (safetensors []RepoFile, configs []RepoFile) {
	for _, f := range files {
		if f.Type != "file" {
			continue
		}

		switch classifyFile(f.Filename()) {
		case fileTypeSafetensors:
			safetensors = append(safetensors, f)
		case fileTypeConfig:
			configs = append(configs, f)
		case fileTypeDiffusersIndex:
			// Skip diffusers index files here since they're handled separately
		case fileTypeUnknown:
			// Skip unknown file types
		}
	}
	return safetensors, configs
}

// TotalSize calculates the total size of files
func TotalSize(files []RepoFile) int64 {
	var total int64
	for _, f := range files {
		total += f.ActualSize()
	}
	return total
}

// isSafetensorsModel checks if the files contain at least one safetensors file
func isSafetensorsModel(files []RepoFile) bool {
	for _, f := range files {
		if f.Type == "file" && classifyFile(f.Filename()) == fileTypeSafetensors {
			return true
		}
	}
	return false
}

// DetectModelType determines if the repository contains an LLM or diffusers model
func DetectModelType(files []RepoFile) ModelType {
	for _, f := range files {
		if f.Type == "file" && f.Path == "model_index.json" {
			return ModelTypeDiffusers
		}
	}
	return ModelTypeLLM
}

// IsDiffusersModel checks if the repository is a diffusers model
func IsDiffusersModel(files []RepoFile) bool {
	return DetectModelType(files) == ModelTypeDiffusers
}

// FilterDiffusersFiles filters repository files for a diffusers model.
// For diffusers models, we need to download:
// - model_index.json
// - All *.safetensors and *.bin files (including in subdirectories)
// - All config.json files (in root and subdirectories)
// - scheduler_config.json, preprocessor_config.json, etc.
func FilterDiffusersFiles(files []RepoFile) (modelFiles []RepoFile, configFiles []RepoFile) {
	for _, f := range files {
		if f.Type != "file" {
			continue
		}

		lower := strings.ToLower(f.Filename())

		// Include model weight files
		if strings.HasSuffix(lower, ".safetensors") || strings.HasSuffix(lower, ".bin") {
			modelFiles = append(modelFiles, f)
			continue
		}

		// Include config files
		if strings.HasSuffix(lower, ".json") ||
			strings.HasSuffix(lower, ".txt") ||
			strings.HasSuffix(lower, ".yaml") ||
			strings.HasSuffix(lower, ".yml") {
			configFiles = append(configFiles, f)
			continue
		}
	}
	return modelFiles, configFiles
}
