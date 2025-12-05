package mutate

import (
	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	ggcr "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-runner/pkg/distribution/types"
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

func ContextSize(mdl types.ModelArtifact, cs int32) types.ModelArtifact {
	return &model{
		base:        mdl,
		contextSize: &cs,
	}
}
