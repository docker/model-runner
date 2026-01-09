package gguf

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
)

// NewModel creates a new GGUF model from a file path.
// It delegates to the unified builder package for model creation.
func NewModel(path string) (*Model, error) {
	// Delegate to builder which handles format detection, shard discovery, and config extraction
	b, err := builder.FromPath(path)
	if err != nil {
		return nil, fmt.Errorf("create model from path: %w", err)
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
