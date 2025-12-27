package safetensors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

var (
	// shardPattern matches safetensors shard filenames like "model-00001-of-00003.safetensors"
	// This pattern assumes 5-digit zero-padded numbering (e.g., 00001-of-00003), which is
	// the most common format used by popular model repositories.
	// The pattern enforces consistent padding width for both the shard number and total count.
	shardPattern = regexp.MustCompile(`^(.+)-(\d{5})-of-(\d{5})\.safetensors$`)
)

// NewModel creates a new safetensors model from one or more safetensors files
// If a sharded model pattern is detected (e.g., model-00001-of-00002.safetensors),
// it will auto-discover all related shards
func NewModel(paths []string) (*Model, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one safetensors file is required")
	}

	// Auto-discover shards if the first path matches the shard pattern
	allPaths, err := discoverSafetensorsShards(paths[0])
	if err != nil {
		return nil, fmt.Errorf("discover safetensors shards: %w", err)
	}
	if len(allPaths) == 0 {
		// Not a sharded file, use provided paths as-is
		allPaths = paths
	}

	layers := make([]oci.Layer, len(allPaths))
	diffIDs := make([]oci.Hash, len(allPaths))

	for i, path := range allPaths {
		layer, layerErr := partial.NewLayer(path, types.MediaTypeSafetensors)
		if layerErr != nil {
			return nil, fmt.Errorf("create safetensors layer from %q: %w", path, layerErr)
		}
		diffID, diffIDErr := layer.DiffID()
		if diffIDErr != nil {
			return nil, fmt.Errorf("get safetensors layer diffID: %w", diffIDErr)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	config, err := configFromFiles(allPaths)
	if err != nil {
		return nil, fmt.Errorf("create config from files: %w", err)
	}

	created := time.Now()
	return &Model{
		BaseModel: partial.BaseModel{
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
			LayerList: layers,
		},
	}, nil
}

// discoverSafetensorsShards attempts to auto-discover all shards for a given safetensors file
// It looks for the pattern: <name>-XXXXX-of-YYYYY.safetensors
// Returns (nil, nil) for single-file models, (paths, nil) for complete shard sets,
// or (nil, error) for incomplete shard sets
func discoverSafetensorsShards(path string) ([]string, error) {
	baseName := filepath.Base(path)
	matches := shardPattern.FindStringSubmatch(baseName)

	if len(matches) != 4 {
		// Not a sharded file, return empty slice with no error
		return nil, nil
	}

	prefix := matches[1]
	totalShards, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("parse shard count: %w", err)
	}

	dir := filepath.Dir(path)
	var shards []string

	// Look for all shards in the same directory
	for i := 1; i <= totalShards; i++ {
		shardName := fmt.Sprintf("%s-%05d-of-%05d.safetensors", prefix, i, totalShards)
		shardPath := filepath.Join(dir, shardName)

		// Check if the file exists
		if _, err := os.Stat(shardPath); err == nil {
			shards = append(shards, shardPath)
		}
	}

	// Return error if we didn't find all expected shards
	if len(shards) != totalShards {
		return nil, fmt.Errorf("incomplete shard set: found %d of %d shards for %s", len(shards), totalShards, baseName)
	}

	// Shards are already in order due to sequential loop
	return shards, nil
}

func configFromFiles(paths []string) (types.Config, error) {
	// Parse the first safetensors file to extract metadata
	if len(paths) == 0 {
		return types.Config{Format: types.FormatSafetensors}, nil
	}

	header, err := ParseSafetensorsHeader(paths[0])
	if err != nil {
		// Continue without metadata if parsing fails
		return types.Config{Format: types.FormatSafetensors}, nil
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

	// Calculate parameters
	params := header.CalculateParameters()

	// Extract architecture from metadata if available
	architecture := ""
	if arch, ok := header.Metadata["architecture"]; ok {
		architecture = fmt.Sprintf("%v", arch)
	}

	return types.Config{
		Format:       types.FormatSafetensors,
		Parameters:   formatParameters(params),
		Quantization: header.GetQuantization(),
		Size:         formatSize(totalSize),
		Architecture: architecture,
		Safetensors:  header.ExtractMetadata(),
	}, nil
}
