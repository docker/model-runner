package gguf

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	mdpartial "github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/types"
)

var _ types.ModelArtifact = &Model{}

type Model struct {
	configFile types.ConfigFile
	layers     []v1.Layer
	manifest   *v1.Manifest
}

func (m *Model) Layers() ([]v1.Layer, error) {
	return m.layers, nil
}

func (m *Model) Size() (int64, error) {
	return partial.Size(m)
}

func (m *Model) ConfigName() (v1.Hash, error) {
	return partial.ConfigName(m)
}

func (m *Model) ConfigFile() (*v1.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

func (m *Model) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

func (m *Model) Manifest() (*v1.Manifest, error) {
	return mdpartial.ManifestForLayers(m)
}

func (m *Model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	for _, l := range m.layers {
		d, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get layer digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer not found")
}

func (m *Model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	for _, l := range m.layers {
		d, err := l.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get layer digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer not found")
}

func (m *Model) RawManifest() ([]byte, error) {
	return partial.RawManifest(m)
}

func (m *Model) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.configFile)
}

func (m *Model) MediaType() (ggcr.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute maniest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *Model) ID() (string, error) {
	return mdpartial.ID(m)
}

func (m *Model) Config() (types.Config, error) {
	return mdpartial.Config(m)
}

func (m *Model) Descriptor() (types.Descriptor, error) {
	return mdpartial.Descriptor(m)
}
