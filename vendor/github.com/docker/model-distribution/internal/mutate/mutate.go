package mutate

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/types"
)

func AppendLayers(mdl types.ModelArtifact, layers ...v1.Layer) types.ModelArtifact {
	return &model{
		base:     mdl,
		appended: layers,
	}
}

func ConfigMediaType(mdl types.ModelArtifact, mt ggcr.MediaType) types.ModelArtifact {
	return &model{
		base:            mdl,
		configMediaType: mt,
	}
}

func ContextSize(mdl types.ModelArtifact, cs uint64) types.ModelArtifact {
	return &model{
		base:        mdl,
		contextSize: &cs,
	}
}
