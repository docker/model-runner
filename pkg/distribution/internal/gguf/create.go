package gguf

import (
	"fmt"
	"time"

	"github.com/docker/model-runner/pkg/distribution/format"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// NewModel creates a new GGUF model from a file path.
// It delegates to the unified format package for shard discovery and config extraction.
func NewModel(path string) (*Model, error) {
	// Get the GGUF format handler
	f, err := format.Get(types.FormatGGUF)
	if err != nil {
		return nil, fmt.Errorf("get format: %w", err)
	}

	// Discover shards using the format package
	shards, err := f.DiscoverShards(path)
	if err != nil {
		return nil, fmt.Errorf("discover shards: %w", err)
	}

	// Create layers
	layers := make([]oci.Layer, len(shards))
	diffIDs := make([]oci.Hash, len(shards))
	for i, shard := range shards {
		layer, err := partial.NewLayer(shard, types.MediaTypeGGUF)
		if err != nil {
			return nil, fmt.Errorf("create gguf layer: %w", err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get gguf layer diffID: %w", err)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	// Extract config using the format package
	config, err := f.ExtractConfig(shards)
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
