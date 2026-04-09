package partial

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// CNCFModel is a model artifact whose config is serialized as a CNCF
// ModelPack config (application/vnd.cncf.model.config.v1+json) and whose
// manifest carries the required artifactType field.
type CNCFModel struct {
	// ModelPackConfig holds the CNCF ModelPack config to be serialized.
	ModelPackConfig modelpack.Model
	// LayerList is the ordered list of OCI layers.
	LayerList []oci.Layer
}

var _ types.ModelArtifact = &CNCFModel{}

// GetManifestOptions implements WithManifestOptions, providing the CNCF
// config media type and required artifact type for the manifest.
func (m *CNCFModel) GetManifestOptions() ManifestOptions {
	return ManifestOptions{
		ConfigMediaType: modelpack.MediaTypeModelConfigV1,
		ArtifactType:    modelpack.ArtifactTypeModelManifest,
	}
}

func (m *CNCFModel) Layers() ([]oci.Layer, error) {
	return m.LayerList, nil
}

func (m *CNCFModel) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.ModelPackConfig)
}

func (m *CNCFModel) Manifest() (*oci.Manifest, error) {
	return ManifestForLayers(m)
}

func (m *CNCFModel) RawManifest() ([]byte, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func (m *CNCFModel) ID() (string, error) {
	return ID(m)
}

func (m *CNCFModel) Config() (types.ModelConfig, error) {
	return &m.ModelPackConfig, nil
}

func (m *CNCFModel) Descriptor() (types.Descriptor, error) {
	// CNCF format stores creation time in ModelDescriptor.CreatedAt.
	return types.Descriptor{Created: m.ModelPackConfig.Descriptor.CreatedAt}, nil
}

func (m *CNCFModel) Size() (int64, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return 0, err
	}
	rawCfg, err := m.RawConfigFile()
	if err != nil {
		return 0, err
	}
	size := int64(len(raw)) + int64(len(rawCfg))
	for _, l := range m.LayerList {
		s, err := l.Size()
		if err != nil {
			return 0, err
		}
		size += s
	}
	return size, nil
}

func (m *CNCFModel) ConfigName() (oci.Hash, error) {
	raw, err := m.RawConfigFile()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *CNCFModel) ConfigFile() (*oci.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for CNCF model")
}

func (m *CNCFModel) Digest() (oci.Hash, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *CNCFModel) MediaType() (oci.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute manifest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *CNCFModel) LayerByDigest(hash oci.Hash) (oci.Layer, error) {
	for _, l := range m.LayerList {
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

func (m *CNCFModel) LayerByDiffID(hash oci.Hash) (oci.Layer, error) {
	for _, l := range m.LayerList {
		d, err := l.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get layer diffID: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer not found")
}
