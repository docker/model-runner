package safetensors

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
)

// NewModel creates a new safetensors model from one or more safetensors files.
// It delegates to the unified builder package for model creation.
func NewModel(paths []string) (*Model, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one safetensors file is required")
	}

	// Delegate to builder which handles format detection, shard discovery, and config extraction
	// Use FromPath for single path (will auto-discover shards)
	// Use FromPaths for multiple explicit paths
	var b *builder.Builder
	var err error

	if len(paths) == 1 {
		b, err = builder.FromPath(paths[0])
	} else {
		b, err = builder.FromPaths(paths)
	}
	if err != nil {
		return nil, fmt.Errorf("create model from paths: %w", err)
	}

	// Get the underlying model and wrap it in our type
	baseModel, ok := b.Model().(*partial.BaseModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type: %T", b.Model())
	}

	return &Model{
		BaseModel: *baseModel,
	}, nil
}
