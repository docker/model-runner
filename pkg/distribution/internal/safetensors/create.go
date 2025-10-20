package safetensors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
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

	layers := make([]v1.Layer, len(allPaths))
	diffIDs := make([]v1.Hash, len(allPaths))

	for i, path := range allPaths {
		layer, err := partial.NewLayer(path, types.MediaTypeSafetensors)
		if err != nil {
			return nil, fmt.Errorf("create safetensors layer from %q: %w", path, err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get safetensors layer diffID: %w", err)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	created := time.Now()
	return &Model{
		configFile: types.ConfigFile{
			Config: configFromFiles(),
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: v1.RootFS{
				Type:    "rootfs",
				DiffIDs: diffIDs,
			},
		},
		layers: layers,
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

func configFromFiles() types.Config {
	return types.Config{
		Format: types.FormatSafetensors,
	}
}
