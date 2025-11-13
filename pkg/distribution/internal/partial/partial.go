package partial

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-runner/pkg/distribution/types"
)

type WithRawConfigFile interface {
	// RawConfigFile returns the serialized bytes of this model's config file.
	RawConfigFile() ([]byte, error)
}

func ConfigFile(i WithRawConfigFile) (*types.ConfigFile, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}
	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal : %w", err)
	}
	return &cf, nil
}

// Config returns the types.Config for the model.
func Config(i WithRawConfigFile) (types.Config, error) {
	cf, err := ConfigFile(i)
	if err != nil {
		return types.Config{}, fmt.Errorf("config file: %w", err)
	}
	return cf.Config, nil
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

func ID(i WithRawManifest) (string, error) {
	digest, err := partial.Digest(i)
	if err != nil {
		return "", fmt.Errorf("get digest: %w", err)
	}
	return digest.String(), nil
}

type WithLayers interface {
	WithRawConfigFile
	Layers() ([]v1.Layer, error)
}

func GGUFPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeGGUF)
}

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

func SafetensorsPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeSafetensors)
}

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

// layerPathsByMediaType is a generic helper function that finds a layer by media type and returns its path
func layerPathsByMediaType(i WithLayers, mediaType ggcr.MediaType) ([]string, error) {
	layers, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}
	var paths []string
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil || mt != mediaType {
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
