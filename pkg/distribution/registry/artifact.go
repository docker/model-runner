package registry

import (
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

var _ types.ModelArtifact = &artifact{}

type artifact struct {
	oci.Image
}

func (a *artifact) ID() (string, error) {
	return partial.ID(a)
}

func (a *artifact) Config() (types.ModelConfig, error) {
	return partial.Config(a)
}

func (a *artifact) Descriptor() (types.Descriptor, error) {
	return partial.Descriptor(a)
}
