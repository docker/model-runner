package format

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// DDUFFormat implements the Format interface for DDUF (Diffusers Unified Format) model files.
// DDUF is a single-file archive format for diffusion models used by HuggingFace Diffusers.
type DDUFFormat struct{}

// init registers the DDUF format implementation.
func init() {
	Register(&DDUFFormat{})
}

// Name returns the format identifier for DDUF.
func (d *DDUFFormat) Name() types.Format {
	return types.FormatDiffusers
}

// MediaType returns the OCI media type for DDUF layers.
func (d *DDUFFormat) MediaType() oci.MediaType {
	return types.MediaTypeDDUF
}

// DiscoverShards finds all DDUF shard files for a model.
// DDUF is a single-file format, so this always returns a slice containing only the input path.
func (d *DDUFFormat) DiscoverShards(path string) ([]string, error) {
	// DDUF files are single archives, not sharded
	return []string{path}, nil
}

// ExtractConfig parses DDUF file(s) and extracts model configuration metadata.
// DDUF files are zip archives containing model config, so we extract what we can.
func (d *DDUFFormat) ExtractConfig(paths []string) (types.Config, error) {
	if len(paths) == 0 {
		return types.Config{Format: types.FormatDiffusers}, nil
	}

	// Calculate total size across all files
	var totalSize int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return types.Config{}, fmt.Errorf("failed to stat file %s: %w", path, err)
		}
		totalSize += info.Size()
	}

	// Extract the filename for metadata
	ddufFile := filepath.Base(paths[0])

	// Return config with diffusers-specific metadata
	// In the future, we could extract model_index.json from the DDUF archive
	// to get architecture details, etc.
	return types.Config{
		Format:       types.FormatDiffusers,
		Architecture: "diffusers",
		Size:         formatSize(totalSize),
		Diffusers: map[string]string{
			"layout":    "dduf",
			"dduf_file": ddufFile,
		},
	}, nil
}
