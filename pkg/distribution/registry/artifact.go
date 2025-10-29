package registry

import (
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

var _ types.ModelArtifact = &artifact{}

type artifact struct {
	v1.Image
}

func (a *artifact) ID() (string, error) {
	return partial.ID(a)
}

func (a *artifact) Config() (types.Config, error) {
	return partial.Config(a)
}

func (a *artifact) Descriptor() (types.Descriptor, error) {
	return partial.Descriptor(a)
}
