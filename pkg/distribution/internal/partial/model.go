package partial

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-runner/pkg/distribution/types"
)

// BaseModel provides a common implementation for model types.
// It can be embedded by specific model format implementations (GGUF, Safetensors, etc.)
type BaseModel struct {
	ModelConfigFile types.ConfigFile
	LayerList       []v1.Layer
}

var _ types.ModelArtifact = &BaseModel{}

func (m *BaseModel) Layers() ([]v1.Layer, error) {
	return m.LayerList, nil
}

func (m *BaseModel) Size() (int64, error) {
	return partial.Size(m)
}

func (m *BaseModel) ConfigName() (v1.Hash, error) {
	return partial.ConfigName(m)
}

func (m *BaseModel) ConfigFile() (*v1.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

func (m *BaseModel) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

func (m *BaseModel) Manifest() (*v1.Manifest, error) {
	return ManifestForLayers(m)
}

func (m *BaseModel) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
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

func (m *BaseModel) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
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
	return partial.RawManifest(m)
}

func (m *BaseModel) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.ModelConfigFile)
}

func (m *BaseModel) MediaType() (ggcr.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute manifest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *BaseModel) ID() (string, error) {
	return ID(m)
}

func (m *BaseModel) Config() (types.Config, error) {
	return Config(m)
}

func (m *BaseModel) Descriptor() (types.Descriptor, error) {
	return Descriptor(m)
}
