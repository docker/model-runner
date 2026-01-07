package partial

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/types"
)

// WithRawConfigFile is an interface for getting raw config file data.
type WithRawConfigFile interface {
	// RawConfigFile returns the serialized bytes of this model's config file.
	RawConfigFile() ([]byte, error)
}

// Config returns the model configuration. Returns *types.Config for Docker format
// or *modelpack.Model for ModelPack format, without any conversion.
func Config(i WithRawConfigFile) (types.ModelConfig, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}

	// ModelPack format: parse directly into modelpack.Model without conversion
	if modelpack.IsModelPackConfig(raw) {
		var mp modelpack.Model
		if err := json.Unmarshal(raw, &mp); err != nil {
			return nil, fmt.Errorf("unmarshal modelpack config: %w", err)
		}
		return &mp, nil
	}

	// Docker format: parse into types.Config
	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cf.Config, nil
}

// ConfigFile returns the full Docker format config file (only for Docker format models).
func ConfigFile(i WithRawConfigFile) (*types.ConfigFile, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}

	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cf, nil
}

// Descriptor returns the types.Descriptor for the model.
func Descriptor(i WithRawConfigFile) (types.Descriptor, error) {
	cf, err := ConfigFile(i)
	if err != nil {
		return types.Descriptor{}, fmt.Errorf("config file: %w", err)
	}
	return cf.Descriptor, nil
}

// WithRawManifest defines the subset of types.Model used by these helper methods
type WithRawManifest interface {
	// RawManifest returns the serialized bytes of this model's manifest file.
	RawManifest() ([]byte, error)
}

// ID returns the ID of the model.
func ID(i WithRawManifest) (string, error) {
	digest, err := partial.Digest(i)
	if err != nil {
		return "", fmt.Errorf("get digest: %w", err)
	}
	return digest.String(), nil
}

// WithLayers is an interface for accessing model layers.
type WithLayers interface {
	WithRawConfigFile
	Layers() ([]v1.Layer, error)
}

// GGUFPaths returns the paths of GGUF layers in the model.
func GGUFPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeGGUF)
}

// MMPROJPath returns the path of the multimodal projector layer in the model.
func MMPROJPath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeMultimodalProjector)
	if err != nil {
		return "", fmt.Errorf("get mmproj layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeMultimodalProjector)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeMultimodalProjector)
	}
	return paths[0], err
}

// ChatTemplatePath returns the path of the chat template layer in the model.
func ChatTemplatePath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeChatTemplate)
	if err != nil {
		return "", fmt.Errorf("get chat template layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeChatTemplate)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeChatTemplate)
	}
	return paths[0], err
}

// SafetensorsPaths returns the paths of safetensors layers in the model.
func SafetensorsPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeSafetensors)
}

// ConfigArchivePath returns the path of the VLLM config archive layer in the model.
func ConfigArchivePath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeVLLMConfigArchive)
	if err != nil {
		return "", fmt.Errorf("get config archive layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeVLLMConfigArchive)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeVLLMConfigArchive)
	}
	return paths[0], err
}

// layerPathsByMediaType is a generic helper function that finds a layer by media type and returns its path.
// Natively supports both Docker and ModelPack media types without any conversion.
func layerPathsByMediaType(i WithLayers, mediaType ggcr.MediaType) ([]string, error) {
	layers, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}
	var paths []string
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			continue
		}
		if !matchesMediaType(mt, mediaType) {
			continue
		}
		layer, ok := l.(*Layer)
		if !ok {
			return nil, fmt.Errorf("%s Layer is not available locally", mediaType)
		}
		paths = append(paths, layer.Path)
	}
	return paths, nil
}

// matchesMediaType checks if a layer media type matches the target type.
// Natively supports both Docker and ModelPack formats without any conversion.
func matchesMediaType(layerMT, targetMT ggcr.MediaType) bool {
	// Exact match
	if layerMT == targetMT {
		return true
	}

	// Native ModelPack support: check equivalent ModelPack types
	//nolint:exhaustive // Only GGUF and Safetensors need cross-format matching
	switch targetMT {
	case types.MediaTypeGGUF:
		// ModelPack GGUF layers also match Docker GGUF target
		return layerMT == ggcr.MediaType(modelpack.MediaTypeWeightGGUF)
	case types.MediaTypeSafetensors:
		// ModelPack safetensors layers also match Docker safetensors target
		return layerMT == ggcr.MediaType(modelpack.MediaTypeWeightSafetensors)
	default:
		// Other media types have no cross-format equivalents
		return false
	}
}

// ManifestForLayers creates a manifest for the given layers.
func ManifestForLayers(i WithLayers) (*v1.Manifest, error) {
	cfgLayer, err := partial.ConfigLayer(i)
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}
	cfgDsc, err := partial.Descriptor(cfgLayer)
	if err != nil {
		return nil, fmt.Errorf("get config descriptor: %w", err)
	}
	cfgDsc.MediaType = types.MediaTypeModelConfigV01

	ls, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	var layers []v1.Descriptor
	for _, l := range ls {
		// Check if this is our Layer type which embeds the full descriptor with annotations
		if layer, ok := l.(*Layer); ok {
			// Use the embedded descriptor directly to preserve annotations
			layers = append(layers, layer.Descriptor)
		} else {
			// Fall back to partial.Descriptor for other layer types
			desc, err := partial.Descriptor(l)
			if err != nil {
				return nil, fmt.Errorf("get layer descriptor: %w", err)
			}
			layers = append(layers, *desc)
		}
	}

	return &v1.Manifest{
		SchemaVersion: 2,
		MediaType:     ggcr.OCIManifestSchema1,
		Config:        *cfgDsc,
		Layers:        layers,
	}, nil
}
