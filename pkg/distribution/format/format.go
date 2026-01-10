// Package format provides a unified interface for handling different model formats.
// It uses the Strategy pattern to encapsulate format-specific behavior while providing
// a common interface for model creation and metadata extraction.
package format

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/files"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/internal/utils"
)

// Format defines the interface for model format-specific operations.
// Implementations handle the differences between GGUF and Safetensors formats.
type Format interface {
	// Name returns the format identifier (e.g., "gguf" or "safetensors")
	Name() types.Format

	// MediaType returns the OCI media type for weight layers of this format
	MediaType() oci.MediaType

	// DiscoverShards finds all shard files for a sharded model given a starting path.
	// For single-file models, it returns a slice containing only the input path.
	// Returns an error if shards are incomplete or cannot be found.
	DiscoverShards(path string) ([]string, error)

	// ExtractConfig parses the weight files and extracts model configuration metadata.
	// This includes parameters count, quantization type, architecture, etc.
	ExtractConfig(paths []string) (types.Config, error)
}

// registry holds all registered format implementations
var registry = make(map[types.Format]Format)

// Register adds a format implementation to the global registry.
// This should be called in init() functions by format implementations.
func Register(f Format) {
	registry[f.Name()] = f
}

// Get returns the format implementation for the given format type.
// Returns an error if the format is not registered.
func Get(name types.Format) (Format, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown format: %s", name)
	}
	return f, nil
}

// DetectFromPath determines the model format based on file extension.
// Returns the appropriate Format implementation or an error if unrecognized.
func DetectFromPath(path string) (Format, error) {
	ft := files.Classify(path)

	switch ft {
	case files.FileTypeGGUF:
		return Get(types.FormatGGUF)
	case files.FileTypeSafetensors:
		return Get(types.FormatSafetensors)
	case files.FileTypeUnknown, files.FileTypeConfig, files.FileTypeLicense, files.FileTypeChatTemplate:
		return nil, fmt.Errorf("unable to detect format from path: %s (file type: %s)", utils.SanitizeForLog(path), ft)
	}
	return nil, fmt.Errorf("unable to detect format from path: %s", utils.SanitizeForLog(path))
}

// DetectFromPaths determines the model format based on a list of file paths.
// All paths must be of the same format. Returns an error if formats are mixed.
func DetectFromPaths(paths []string) (Format, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths provided")
	}

	// Detect format from first path
	format, err := DetectFromPath(paths[0])
	if err != nil {
		return nil, err
	}

	// Verify all paths are the same format
	expectedName := format.Name()
	for _, p := range paths[1:] {
		f, err := DetectFromPath(p)
		if err != nil {
			return nil, err
		}
		if f.Name() != expectedName {
			return nil, fmt.Errorf("mixed formats detected: %s and %s", expectedName, f.Name())
		}
	}

	return format, nil
}
