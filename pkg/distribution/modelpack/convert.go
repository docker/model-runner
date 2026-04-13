package modelpack

import (
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/files"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/opencontainers/go-digest"
)

// LayerKind is a semantic classification of a model artifact layer.
// It maps to specific CNCF model-spec media types.
type LayerKind int

const (
	// KindWeight is a primary model weight file (GGUF, safetensors, DDUF,
	// mmproj, etc.).
	KindWeight LayerKind = iota
	// KindWeightConfig is a weight config file: tokenizer.json, config.json,
	// vLLM config archives, chat templates, etc.
	KindWeightConfig
	// KindDoc is a documentation file: README.md, LICENSE, etc.
	KindDoc
)

// ClassifyLayer determines the CNCF model-spec LayerKind for a layer.
// Resolution order:
//  1. Explicit Docker semantic media types (most specific).
//  2. Filepath/annotation heuristics for ambiguous media types.
//  3. Docker media type fallback.
func ClassifyLayer(dockerMT oci.MediaType, path string) LayerKind {
	switch dockerMT { //nolint:exhaustive // Only Docker and CNCF semantic media types are classified; OCI standard types fall through to filepath heuristics.
	// Docker-format documentation types.
	case types.MediaTypeLicense, MediaTypeDocRaw:
		return KindDoc
	// Docker-format weight config types.
	case types.MediaTypeChatTemplate, types.MediaTypeVLLMConfigArchive, types.MediaTypeModelFile, MediaTypeWeightConfigRaw:
		return KindWeightConfig
	// Docker-format weight types.
	case types.MediaTypeMultimodalProjector:
		return KindWeight
	case types.MediaTypeGGUF, types.MediaTypeSafetensors, types.MediaTypeDDUF:
		return KindWeight
	// CNCF model-spec weight types (including legacy typed media types).
	case MediaTypeWeightRaw, MediaTypeWeightGGUF, MediaTypeWeightSafetensors:
		return KindWeight
	}

	// Use filepath heuristics for ambiguous or unknown media types.
	if path != "" {
		return classifyByPath(path)
	}

	// Default: treat unknown media types (without filepath hints) as weight
	// config. This is intentional for the directory-based packaging flow
	// where ambiguous files (tokenizer.json, config.json, etc.) are common
	// and typically carry configuration rather than model weights. All known
	// weight media types — both Docker (MediaTypeGGUF, MediaTypeSafetensors,
	// etc.) and CNCF (MediaTypeWeightRaw, etc.) — are handled explicitly in
	// the switch above, so this fallback only triggers for truly unrecognized
	// media types.
	return KindWeightConfig
}

// classifyByPath classifies a file as a LayerKind based on its path/name.
func classifyByPath(path string) LayerKind {
	ft := files.Classify(path)
	switch ft {
	case files.FileTypeGGUF, files.FileTypeSafetensors, files.FileTypeDDUF:
		return KindWeight
	case files.FileTypeLicense:
		return KindDoc
	case files.FileTypeChatTemplate:
		return KindWeightConfig
	case files.FileTypeUnknown:
		return KindWeightConfig
	case files.FileTypeConfig:
		// .md files are documentation, not weight config.
		if strings.ToLower(filepath.Ext(path)) == ".md" {
			return KindDoc
		}
		return KindWeightConfig
	default:
		return KindWeightConfig
	}
}

// LayerKindToMediaType maps a LayerKind to the CNCF model-spec raw media type.
func LayerKindToMediaType(kind LayerKind) oci.MediaType {
	switch kind {
	case KindWeight:
		return MediaTypeWeightRaw
	case KindDoc:
		return MediaTypeDocRaw
	case KindWeightConfig:
		return MediaTypeWeightConfigRaw
	}
	return MediaTypeWeightConfigRaw
}

// MapLayerMediaType returns the CNCF model-spec media type for the given
// Docker layer media type and optional filepath annotation.
func MapLayerMediaType(dockerMT oci.MediaType, path string) oci.MediaType {
	return LayerKindToMediaType(ClassifyLayer(dockerMT, path))
}

// DockerConfigToModelPack converts a Docker-format model config into a
// CNCF ModelPack Model config. The diffIDs should already be in
// digest.Digest ("algorithm:hex") format.
func DockerConfigToModelPack(
	cfg types.Config,
	desc types.Descriptor,
	diffIDs []digest.Digest,
) Model {
	// Preserve determinism by propagating desc.Created directly.
	// Callers that require a concrete timestamp should set desc.Created
	// explicitly before calling this function.
	return Model{
		Descriptor: ModelDescriptor{
			CreatedAt: desc.Created,
			// Map architecture to family as the closest available field.
			Family: cfg.Architecture,
		},
		Config: ModelConfig{
			Architecture: cfg.Architecture,
			Format:       string(cfg.Format),
			ParamSize:    normalizeParamSize(cfg.Parameters),
			Quantization: cfg.Quantization,
		},
		ModelFS: ModelFS{
			Type:    "layers",
			DiffIDs: diffIDs,
		},
	}
}

// normalizeParamSize lowercases a Docker-format parameters string for use
// as the model-spec paramSize field (e.g. "8.03B" → "8.03b", "70B" → "70b").
// Returns empty string if s is empty.
func normalizeParamSize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s)
}
