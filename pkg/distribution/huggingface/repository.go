package huggingface

import (
	"path"
	"sort"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/files"
)

const (
	// DefaultGGUFQuantization is the preferred quantization when "latest" is requested
	DefaultGGUFQuantization = "Q4_K_M"
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
// Returns weight files (GGUF or SafeTensors) and config files separately
func FilterModelFiles(repoFiles []RepoFile) (weights []RepoFile, configs []RepoFile) {
	for _, f := range repoFiles {
		if f.Type != "file" {
			continue
		}

		switch ft := files.Classify(f.Filename()); ft {
		case files.FileTypeSafetensors, files.FileTypeGGUF:
			weights = append(weights, f)
		case files.FileTypeConfig, files.FileTypeChatTemplate:
			configs = append(configs, f)
		case files.FileTypeUnknown, files.FileTypeLicense, files.FileTypeDDUF:
			// Skip these file types
		}
	}
	return weights, configs
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

// isGGUFModel checks if the files contain at least one GGUF file
func isGGUFModel(repoFiles []RepoFile) bool {
	for _, f := range repoFiles {
		if f.Type == "file" && files.Classify(f.Filename()) == files.FileTypeGGUF {
			return true
		}
	}
	return false
}

// SelectGGUFFiles selects GGUF files based on the requested quantization.
// For GGUF repos with multiple quantization variants:
// - If requestedQuant matches a known quantization (e.g., "Q4_K_M"), select files with that quantization
// - If requestedQuant is empty, "latest", or "main", prefer Q4_K_M, then fall back to first GGUF
// - Handles sharded GGUF files (selects all shards of the chosen quantization)
// - Also selects mmproj files for multimodal models (prefers F16)
func SelectGGUFFiles(ggufFiles []RepoFile, requestedQuant string) (selected []RepoFile, mmproj *RepoFile) {
	if len(ggufFiles) == 0 {
		return nil, nil
	}

	// Separate mmproj files from model files
	var modelFiles []RepoFile
	var mmprojFiles []RepoFile

	for _, f := range ggufFiles {
		filename := f.Filename()
		if isMMProjFile(filename) {
			mmprojFiles = append(mmprojFiles, f)
		} else {
			modelFiles = append(modelFiles, f)
		}
	}

	// Select mmproj file (prefer F16)
	mmproj = selectMMProj(mmprojFiles)

	// If only one model file, return it
	if len(modelFiles) == 1 {
		return modelFiles, mmproj
	}

	// Normalize requested quantization
	quant := normalizeQuantization(requestedQuant)

	// Try to find files matching the requested quantization
	if quant != "" {
		matching := filterByQuantization(modelFiles, quant)
		if len(matching) > 0 {
			return matching, mmproj
		}
	}

	// Fall back to default quantization (Q4_K_M)
	defaultMatching := filterByQuantization(modelFiles, DefaultGGUFQuantization)
	if len(defaultMatching) > 0 {
		return defaultMatching, mmproj
	}

	// No specific quantization found - return the first file (or sharded set)
	// Sort by filename to ensure consistent selection
	first := selectFirstGGUF(modelFiles)
	return first, mmproj
}

// normalizeQuantization normalizes the quantization string
// Returns empty string for "latest" or "main" (meaning use default)
func normalizeQuantization(quant string) string {
	if quant == "" || quant == "latest" || quant == "main" {
		return ""
	}
	return quant
}

// filterByQuantization filters GGUF files by quantization type
// Handles both single files and sharded files
func filterByQuantization(modelFiles []RepoFile, quant string) []RepoFile {
	var matching []RepoFile

	for _, f := range modelFiles {
		filename := f.Filename()
		if containsQuantization(filename, quant) {
			matching = append(matching, f)
		}
	}

	return matching
}

// containsQuantization checks if a filename contains the specified quantization
// Matches patterns like "model-Q4_K_M.gguf" or "model-Q4_K_M-00001-of-00003.gguf"
func containsQuantization(filename, quant string) bool {
	// Case-insensitive comparison
	filenameLower := strings.ToLower(filename)
	quantLower := strings.ToLower(quant)

	// Remove .gguf extension for cleaner matching
	if hasSuffix(filenameLower, ".gguf") {
		filenameLower = filenameLower[:len(filenameLower)-5]
	}

	// Common patterns:
	// - "model-Q4_K_M" -> ends with "-Q4_K_M" or "-Q4_K_M-00001-of-00003"
	// - "model.Q4_K_M" -> ends with ".Q4_K_M"
	// - "Llama-3.2-1B-Instruct-Q4_K_M" -> ends with "-Q4_K_M"

	// Check if the quantization appears after a separator (-, ., _) and is followed by
	// either end of string or another separator
	separators := []string{"-", ".", "_"}
	for _, sep := range separators {
		pattern := sep + quantLower
		idx := strings.Index(filenameLower, pattern)
		if idx >= 0 {
			// Check what comes after the quantization
			afterIdx := idx + len(pattern)
			if afterIdx == len(filenameLower) {
				// Quantization is at end of filename (after removing .gguf)
				return true
			}
			// Check if followed by a separator (e.g., "-00001-of-00003")
			if afterIdx < len(filenameLower) {
				nextChar := filenameLower[afterIdx]
				if nextChar == '-' || nextChar == '.' || nextChar == '_' {
					return true
				}
			}
		}
	}

	return false
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// selectFirstGGUF selects the first GGUF file (handling sharded files)
func selectFirstGGUF(modelFiles []RepoFile) []RepoFile {
	if len(modelFiles) == 0 {
		return nil
	}

	// Sort by filename for consistent ordering
	sorted := make([]RepoFile, len(modelFiles))
	copy(sorted, modelFiles)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Filename() < sorted[j].Filename() })

	// Get the first file
	first := sorted[0]

	// Check if it's a sharded file
	if isShardedFile(first.Filename()) {
		// Find all shards with the same prefix
		return findAllShards(sorted, first.Filename())
	}

	return []RepoFile{first}
}

// isShardedFile checks if a filename follows the sharded pattern
// e.g., "model-00001-of-00003.gguf"
func isShardedFile(filename string) bool {
	// Delegate to indexOfShardPattern so shard detection is precise and consistent
	return indexOfShardPattern(filename) >= 0
}

// findAllShards finds all shards that belong to the same model
func findAllShards(files []RepoFile, firstShard string) []RepoFile {
	// Extract the base prefix (everything before the shard number)
	// e.g., "model-00001-of-00003.gguf" -> "model"
	prefix := extractShardPrefix(firstShard)

	var shards []RepoFile
	for _, f := range files {
		if strings.HasPrefix(f.Filename(), prefix) && isShardedFile(f.Filename()) {
			shards = append(shards, f)
		}
	}

	return shards
}

// extractShardPrefix extracts the model name prefix from a sharded filename
func extractShardPrefix(filename string) string {
	// Find "-00001-of-" or similar pattern and return everything before it
	idx := indexOfShardPattern(filename)
	if idx > 0 {
		return filename[:idx]
	}
	return filename
}

// isMMProjFile checks if a file is a multimodal projector file
func isMMProjFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.Contains(lower, "mmproj")
}

// selectMMProj selects the best mmproj file, preferring F16
func selectMMProj(mmprojFiles []RepoFile) *RepoFile {
	if len(mmprojFiles) == 0 {
		return nil
	}

	// Prefer F16 over other formats
	for i := range mmprojFiles {
		filename := strings.ToLower(mmprojFiles[i].Filename())
		if strings.Contains(filename, "f16") {
			return &mmprojFiles[i]
		}
	}

	// Fall back to first mmproj file
	return &mmprojFiles[0]
}

func indexOfShardPattern(filename string) int {
	// Look for pattern like "-00001-of-" or "-00002-of-"
	for i := 0; i < len(filename)-10; i++ {
		if filename[i] == '-' &&
			filename[i+1] >= '0' && filename[i+1] <= '9' &&
			filename[i+2] >= '0' && filename[i+2] <= '9' &&
			filename[i+3] >= '0' && filename[i+3] <= '9' &&
			filename[i+4] >= '0' && filename[i+4] <= '9' &&
			filename[i+5] >= '0' && filename[i+5] <= '9' &&
			filename[i+6] == '-' &&
			filename[i+7] == 'o' &&
			filename[i+8] == 'f' &&
			filename[i+9] == '-' {
			return i
		}
	}
	return -1
}
