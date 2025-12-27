package mutate

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

type model struct {
	base            types.ModelArtifact
	appended        []oci.Layer
	configMediaType oci.MediaType
	contextSize     *int32
}

func (m *model) Descriptor() (types.Descriptor, error) {
	return partial.Descriptor(m.base)
}

func (m *model) ID() (string, error) {
	return partial.ID(m)
}

func (m *model) Config() (types.ModelConfig, error) {
	return partial.Config(m)
}

func (m *model) MediaType() (oci.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute maniest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *model) Size() (int64, error) {
	return oci.Size(m)
}

func (m *model) ConfigName() (oci.Hash, error) {
	return oci.ConfigName(m)
}

func (m *model) ConfigFile() (*oci.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

func (m *model) Digest() (oci.Hash, error) {
	return oci.Digest(m)
}

func (m *model) RawManifest() ([]byte, error) {
	return oci.RawManifest(m)
}

func (m *model) LayerByDigest(hash oci.Hash) (oci.Layer, error) {
	ls, err := m.Layers()
	if err != nil {
		return nil, err
	}
	for _, l := range ls {
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

func (m *model) LayerByDiffID(hash oci.Hash) (oci.Layer, error) {
	ls, err := m.Layers()
	if err != nil {
		return nil, err
	}
	for _, l := range ls {
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

func (m *model) Layers() ([]oci.Layer, error) {
	ls, err := m.base.Layers()
	if err != nil {
		return nil, err
	}
	return append(ls, m.appended...), nil
}

func (m *model) Manifest() (*oci.Manifest, error) {
	manifest, err := partial.ManifestForLayers(m)
	if err != nil {
		return nil, err
	}
	if m.configMediaType != "" {
		manifest.Config.MediaType = m.configMediaType
	}
	return manifest, nil
}

func (m *model) RawConfigFile() ([]byte, error) {
	cf, err := partial.ConfigFile(m.base)
	if err != nil {
		return nil, err
	}
	for _, l := range m.appended {
		diffID, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		cf.RootFS.DiffIDs = append(cf.RootFS.DiffIDs, diffID)
	}
	if m.contextSize != nil {
		cf.Config.ContextSize = m.contextSize
	}
	raw, err := json.Marshal(cf)
	if err != nil {
		return nil, err
	}
	return raw, err
}
