package safetensors

import (
	"fmt"
	"time"

	"github.com/docker/model-runner/pkg/distribution/format"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// NewModel creates a new safetensors model from one or more safetensors files.
// It delegates to the unified format package for shard discovery and config extraction.
func NewModel(paths []string) (*Model, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one safetensors file is required")
	}

	// Get the Safetensors format handler
	f, err := format.Get(types.FormatSafetensors)
	if err != nil {
		return nil, fmt.Errorf("get format: %w", err)
	}

	// Auto-discover shards if the first path matches the shard pattern
	allPaths, err := f.DiscoverShards(paths[0])
	if err != nil {
		return nil, fmt.Errorf("discover safetensors shards: %w", err)
	}
	if len(allPaths) == 1 && len(paths) > 1 {
		// Not a sharded file but multiple paths provided, use provided paths as-is
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

	// Extract config using the format package
	config, err := f.ExtractConfig(allPaths)
	if err != nil {
		return nil, fmt.Errorf("extract config: %w", err)
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
