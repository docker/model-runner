package huggingface

import (
	"path"

	"github.com/docker/model-runner/pkg/distribution/files"
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

// FilterModelFiles filters repository files to only include files needed for model-runner
// Returns safetensors files and config files separately
func FilterModelFiles(repoFiles []RepoFile) (safetensors []RepoFile, configs []RepoFile) {
	for _, f := range repoFiles {
		if f.Type != "file" {
			continue
		}

		switch ft := files.Classify(f.Filename()); ft {
		case files.FileTypeSafetensors:
			safetensors = append(safetensors, f)
		case files.FileTypeConfig, files.FileTypeChatTemplate:
			configs = append(configs, f)
		case files.FileTypeUnknown, files.FileTypeGGUF, files.FileTypeLicense:
			// Skip these file types
		}
	}
	return safetensors, configs
}

// TotalSize calculates the total size of files
func TotalSize(repoFiles []RepoFile) int64 {
	var total int64
	for _, f := range repoFiles {
		total += f.ActualSize()
	}
	return total
}

// isSafetensorsModel checks if the files contain at least one safetensors file
func isSafetensorsModel(repoFiles []RepoFile) bool {
	for _, f := range repoFiles {
		if f.Type == "file" && files.Classify(f.Filename()) == files.FileTypeSafetensors {
			return true
		}
	}
	return false
}
