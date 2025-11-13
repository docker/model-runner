package safetensors

import (
	mdpartial "github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/types"
)

var _ types.ModelArtifact = &Model{}

// Model represents a Safetensors model and embeds BaseModel for common functionality
type Model struct {
	mdpartial.BaseModel
}
