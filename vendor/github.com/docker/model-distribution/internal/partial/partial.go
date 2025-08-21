package partial

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/types"
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

func GGUFPath(i WithLayers) (string, error) {
	return layerPathByMediaType(i, types.MediaTypeGGUF)
}

func MMPROJPath(i WithLayers) (string, error) {
	return layerPathByMediaType(i, types.MediaTypeMultimodalProjector)
}

// layerPathByMediaType is a generic helper function that finds a layer by media type and returns its path
func layerPathByMediaType(i WithLayers, mediaType ggcr.MediaType) (string, error) {
	layers, err := i.Layers()
	if err != nil {
		return "", fmt.Errorf("get layers: %w", err)
	}
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil || mt != mediaType {
			continue
		}
		layer, ok := l.(*Layer)
		if !ok {
			return "", fmt.Errorf("%s Layer is not available locally", mediaType)
		}
		return layer.Path, nil
	}
	return "", fmt.Errorf("model does not contain a %s layer", mediaType)
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
		desc, err := partial.Descriptor(l)
		if err != nil {
			return nil, fmt.Errorf("get layer descriptor: %w", err)
		}
		layers = append(layers, *desc)
	}

	return &v1.Manifest{
		SchemaVersion: 2,
		MediaType:     ggcr.OCIManifestSchema1,
		Config:        *cfgDsc,
		Layers:        layers,
	}, nil
}
