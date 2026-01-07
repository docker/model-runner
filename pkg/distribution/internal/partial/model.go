package partial

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/types"
)

// BaseModel provides a common implementation for model types.
// It can be embedded by specific model format implementations (GGUF, Safetensors, etc.)
type BaseModel struct {
	ModelConfigFile types.ConfigFile
	LayerList       []v1.Layer
}

var _ types.ModelArtifact = &BaseModel{}

// Layers returns the layers of the model.
func (m *BaseModel) Layers() ([]v1.Layer, error) {
	return m.LayerList, nil
}

// Size returns the total size of the model.
func (m *BaseModel) Size() (int64, error) {
	return partial.Size(m)
}

// ConfigName returns the hash of the model's config file.
func (m *BaseModel) ConfigName() (v1.Hash, error) {
	return partial.ConfigName(m)
}

// ConfigFile returns the model's config file.
func (m *BaseModel) ConfigFile() (*v1.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

// Digest returns the digest of the model.
func (m *BaseModel) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

// Manifest returns the manifest of the model.
func (m *BaseModel) Manifest() (*v1.Manifest, error) {
	return ManifestForLayers(m)
}

// LayerByDigest returns the layer with the given digest.
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

// LayerByDiffID returns the layer with the given diff ID.
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

// RawManifest returns the raw manifest of the model.
func (m *BaseModel) RawManifest() ([]byte, error) {
	return partial.RawManifest(m)
}

// RawConfigFile returns the raw config file of the model.
func (m *BaseModel) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.ModelConfigFile)
}

// MediaType returns the media type of the model.
func (m *BaseModel) MediaType() (ggcr.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute manifest: %w", err)
	}
	return manifest.MediaType, nil
}

// ID returns the ID of the model.
func (m *BaseModel) ID() (string, error) {
	return ID(m)
}

// Config returns the configuration of the model.
func (m *BaseModel) Config() (types.ModelConfig, error) {
	return Config(m)
}

// Descriptor returns the descriptor of the model.
func (m *BaseModel) Descriptor() (types.Descriptor, error) {
	return Descriptor(m)
}
