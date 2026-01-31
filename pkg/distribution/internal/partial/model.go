package partial

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// BaseModel provides a common implementation for model types.
// It can be embedded by specific model format implementations (GGUF, Safetensors, etc.)
type BaseModel struct {
	ModelConfigFile types.ConfigFile
	LayerList       []oci.Layer
	// ConfigMediaType specifies the media type for the config descriptor.
	// If empty, defaults to MediaTypeModelConfigV01 for backward compatibility.
	// Set to MediaTypeModelConfigV02 for layer-per-file packaging (FromDirectory).
	ConfigMediaType oci.MediaType
}

var _ types.ModelArtifact = &BaseModel{}

func (m *BaseModel) Layers() ([]oci.Layer, error) {
	return m.LayerList, nil
}

func (m *BaseModel) Size() (int64, error) {
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

func (m *BaseModel) ConfigName() (oci.Hash, error) {
	raw, err := m.RawConfigFile()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *BaseModel) ConfigFile() (*oci.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

func (m *BaseModel) Digest() (oci.Hash, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *BaseModel) Manifest() (*oci.Manifest, error) {
	return ManifestForLayers(m)
}

func (m *BaseModel) LayerByDigest(hash oci.Hash) (oci.Layer, error) {
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

func (m *BaseModel) LayerByDiffID(hash oci.Hash) (oci.Layer, error) {
	for _, l := range m.LayerList {
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

func (m *BaseModel) RawManifest() ([]byte, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func (m *BaseModel) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.ModelConfigFile)
}

func (m *BaseModel) MediaType() (oci.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute manifest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *BaseModel) ID() (string, error) {
	return ID(m)
}

func (m *BaseModel) Config() (types.ModelConfig, error) {
	return Config(m)
}

func (m *BaseModel) Descriptor() (types.Descriptor, error) {
	return Descriptor(m)
}

// GetConfigMediaType returns the config media type for the model.
// If not set, returns empty string and ManifestForLayers will default to V0.1.
func (m *BaseModel) GetConfigMediaType() oci.MediaType {
	return m.ConfigMediaType
}
