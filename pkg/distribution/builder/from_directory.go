package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/distribution/files"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// DirectoryOptions configures the behavior of FromDirectory.
type DirectoryOptions struct {
	// Exclusions is a list of patterns to exclude from packaging.
	// Patterns can be:
	//   - Directory names (e.g., ".git", "__pycache__") - excludes the entire directory
	//   - File names (e.g., "README.md") - excludes files with this exact name
	//   - Glob patterns (e.g., "*.log", "*.tmp") - excludes files matching the pattern
	//   - Paths with slashes (e.g., "logs/debug.log") - excludes specific paths
	Exclusions []string
}

// DirectoryOption is a functional option for configuring FromDirectory.
type DirectoryOption func(*DirectoryOptions)

// WithExclusions specifies patterns to exclude from packaging.
// Patterns can be directory names, file names, glob patterns, or specific paths.
//
// Examples:
//
//	WithExclusions(".git", "__pycache__")           // Exclude directories
//	WithExclusions("README.md", "CHANGELOG.md")     // Exclude specific files
//	WithExclusions("*.log", "*.tmp")                // Exclude by pattern
//	WithExclusions("logs/", "cache/")               // Exclude directories (trailing slash)
func WithExclusions(patterns ...string) DirectoryOption {
	return func(opts *DirectoryOptions) {
		opts.Exclusions = append(opts.Exclusions, patterns...)
	}
}

// FromDirectory creates a Builder from a directory containing model files.
// It recursively scans the directory and adds each non-hidden file as a separate layer.
// Each layer's filepath annotation preserves the relative path from the directory root.
//
// The directory structure is fully preserved, enabling support for nested HuggingFace models
// like Qwen3-TTS that have subdirectories (text_encoder/, vae/, etc.).
//
// Example directory structure:
//
//	model_dir/
//	  config.json               -> layer with annotation "config.json"
//	  model.safetensors         -> layer with annotation "model.safetensors"
//	  text_encoder/
//	    config.json             -> layer with annotation "text_encoder/config.json"
//	    model.safetensors       -> layer with annotation "text_encoder/model.safetensors"
//
// Example with exclusions:
//
//	builder.FromDirectory(dirPath, builder.WithExclusions(".git", "__pycache__", "*.log"))
func FromDirectory(dirPath string, opts ...DirectoryOption) (*Builder, error) {
	// Apply options
	options := &DirectoryOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Verify directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	var layers []oci.Layer
	var diffIDs []oci.Hash
	var detectedFormat types.Format
	var weightFiles []string

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

		// Calculate relative path from the directory root
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("compute relative path: %w", err)
		}

		// Check exclusions
		if shouldExclude(info, relPath, options.Exclusions) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories (but continue walking into them)
		if info.IsDir() {
			return nil
		}

		// Skip symlinks for security
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Classify the file to determine media type
		fileType := files.Classify(path)
		mediaType := fileTypeToMediaType(fileType)

		// Track format from weight files
		switch fileType {
		case files.FileTypeSafetensors:
			if detectedFormat == "" {
				detectedFormat = types.FormatSafetensors
			}
			weightFiles = append(weightFiles, path)
		case files.FileTypeGGUF:
			if detectedFormat == "" {
				detectedFormat = types.FormatGGUF
			}
			weightFiles = append(weightFiles, path)
		case files.FileTypeDDUF:
			if detectedFormat == "" {
				detectedFormat = types.FormatDiffusers
			}
			weightFiles = append(weightFiles, path)
		case files.FileTypeUnknown:
		case files.FileTypeConfig:
		case files.FileTypeLicense:
		case files.FileTypeChatTemplate:
		}

		// Create layer with relative path annotation
		layer, err := partial.NewLayerWithRelativePath(path, relPath, mediaType)
		if err != nil {
			return fmt.Errorf("create layer for %q: %w", relPath, err)
		}

		diffID, err := layer.DiffID()
		if err != nil {
			return fmt.Errorf("get diffID for %q: %w", relPath, err)
		}

		layers = append(layers, layer)
		diffIDs = append(diffIDs, diffID)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no files found in directory: %s", dirPath)
	}

	if len(weightFiles) == 0 {
		return nil, fmt.Errorf("no weight files (safetensors, GGUF, or DDUF) found in directory: %s", dirPath)
	}

	// Build config - use the first weight file for metadata extraction
	config := types.Config{
		Format: detectedFormat,
	}

	// TODO: Extract additional metadata from weight files if needed
	// For safetensors, we might want to read config.json from the directory

	// Build the model with V0.2 config (layer-per-file with annotations)
	created := time.Now()
	mdl := &partial.BaseModel{
		ModelConfigFile: types.ConfigFile{
			Config: config,
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: oci.RootFS{
				Type:    "rootfs",
				DiffIDs: diffIDs,
			},
		},
		LayerList:       layers,
		ConfigMediaType: types.MediaTypeModelConfigV02, // V0.2: layer-per-file with filepath annotations
	}

	return &Builder{
		model: mdl,
	}, nil
}

// shouldExclude checks if a file or directory should be excluded based on the exclusion patterns.
func shouldExclude(info os.FileInfo, relPath string, exclusions []string) bool {
	if len(exclusions) == 0 {
		return false
	}

	name := info.Name()
	// Normalize path separators for cross-platform matching
	normalizedRelPath := filepath.ToSlash(relPath)

	for _, pattern := range exclusions {
		// Normalize the pattern
		pattern = filepath.ToSlash(pattern)

		// Pattern ends with "/" - match directories only
		if strings.HasSuffix(pattern, "/") {
			if info.IsDir() {
				dirPattern := strings.TrimSuffix(pattern, "/")
				// Match directory name
				if name == dirPattern {
					return true
				}
				// Match full path
				if normalizedRelPath == dirPattern || strings.HasPrefix(normalizedRelPath, dirPattern+"/") {
					return true
				}
			}
			continue
		}

		// Pattern contains "/" - treat as path match
		if strings.Contains(pattern, "/") {
			// Exact path match
			if normalizedRelPath == pattern {
				return true
			}
			// Directory path prefix match
			if info.IsDir() && strings.HasPrefix(normalizedRelPath+"/", pattern+"/") {
				return true
			}
			// File inside excluded directory
			if strings.HasPrefix(normalizedRelPath, pattern+"/") {
				return true
			}
			continue
		}

		// Pattern contains glob characters - use glob matching
		if strings.ContainsAny(pattern, "*?[]") {
			matched, err := filepath.Match(pattern, name)
			if err == nil && matched {
				return true
			}
			continue
		}

		// Simple name match (works for both files and directories)
		if name == pattern {
			return true
		}
	}

	return false
}

// fileTypeToMediaType converts a FileType to the corresponding OCI MediaType
func fileTypeToMediaType(ft files.FileType) oci.MediaType {
	switch ft {
	case files.FileTypeGGUF:
		return types.MediaTypeGGUF
	case files.FileTypeSafetensors:
		return types.MediaTypeSafetensors
	case files.FileTypeDDUF:
		return types.MediaTypeDDUF
	case files.FileTypeLicense:
		return types.MediaTypeLicense
	case files.FileTypeChatTemplate:
		return types.MediaTypeChatTemplate
	case files.FileTypeConfig:
		return types.MediaTypeModelFile
	default:
		// For unknown files, use the generic model file type
		return types.MediaTypeModelFile
	}
}

// WithFileLayer adds an individual file layer with a relative path annotation.
// This is useful for adding files that should be extracted to a specific path.
func (b *Builder) WithFileLayer(absPath, relPath string) (*Builder, error) {
	// Classify the file to determine media type
	fileType := files.Classify(absPath)
	mediaType := fileTypeToMediaType(fileType)

	layer, err := partial.NewLayerWithRelativePath(absPath, relPath, mediaType)
	if err != nil {
		return nil, fmt.Errorf("file layer from %q: %w", absPath, err)
	}

	return &Builder{
		model:          mutate.AppendLayers(b.model, layer),
		originalLayers: b.originalLayers,
	}, nil
}
