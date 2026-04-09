package mutate

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

type model struct {
	base            types.ModelArtifact
	appended        []oci.Layer
	configMediaType oci.MediaType
	artifactType    string
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

// GetManifestOptions implements partial.WithManifestOptions and propagates
// the manifest options from the base model, applying any overrides set on
// this wrapper. This ensures artifactType and config media type survive
// through arbitrarily deep mutate chains.
func (m *model) GetManifestOptions() partial.ManifestOptions {
	// Start with base model's manifest options.
	var opts partial.ManifestOptions
	if base, ok := m.base.(partial.WithManifestOptions); ok {
		opts = base.GetManifestOptions()
	} else if cmt, ok := m.base.(partial.WithConfigMediaType); ok {
		opts.ConfigMediaType = cmt.GetConfigMediaType()
	}
	// Apply overrides set on this wrapper.
	if m.configMediaType != "" {
		opts.ConfigMediaType = m.configMediaType
	}
	if m.artifactType != "" {
		opts.ArtifactType = m.artifactType
	}
	return opts
}

func (m *model) Manifest() (*oci.Manifest, error) {
	// ManifestForLayers reads GetManifestOptions() via the interface, so
	// config media type and artifact type are handled there.
	return partial.ManifestForLayers(m)
}

// isCNCFBase reports whether the base model chain produces CNCF ModelPack config.
func (m *model) isCNCFBase() bool {
	raw, err := m.base.RawConfigFile()
	if err != nil {
		return false
	}
	return modelpack.IsModelPackConfig(raw)
}

func (m *model) RawConfigFile() ([]byte, error) {
	if m.isCNCFBase() {
		return m.rawCNCFConfigFile()
	}
	return m.rawDockerConfigFile()
}

// rawDockerConfigFile builds the Docker-format config file, appending DiffIDs
// and optionally setting context size.
func (m *model) rawDockerConfigFile() ([]byte, error) {
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
	return json.Marshal(cf)
}

// rawCNCFConfigFile builds the CNCF ModelPack config file, appending DiffIDs
// to ModelFS. Context size is not supported in the CNCF format.
func (m *model) rawCNCFConfigFile() ([]byte, error) {
	raw, err := m.base.RawConfigFile()
	if err != nil {
		return nil, err
	}
	var mp modelpack.Model
	if err := json.Unmarshal(raw, &mp); err != nil {
		return nil, fmt.Errorf("unmarshal cncf config: %w", err)
	}
	for _, l := range m.appended {
		diffID, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		// Convert oci.Hash to digest.Digest ("algorithm:hex" string form).
		mp.ModelFS.DiffIDs = append(
			mp.ModelFS.DiffIDs,
			modelpack.HashToDigest(diffID.String()),
		)
	}
	return json.Marshal(mp)
}
