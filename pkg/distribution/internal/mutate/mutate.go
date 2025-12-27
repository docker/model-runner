package mutate

import (
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func AppendLayers(mdl types.ModelArtifact, layers ...oci.Layer) types.ModelArtifact {
	return &model{
		base:     mdl,
		appended: layers,
	}
}

func ConfigMediaType(mdl types.ModelArtifact, mt oci.MediaType) types.ModelArtifact {
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
