package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	mdpartial "github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	mdtypes "github.com/docker/model-runner/pkg/distribution/types"
)

var _ oci.Image = &Model{}

type Model struct {
	rawManifest   []byte
	manifest      *oci.Manifest
	rawConfigFile []byte
	layers        []oci.Layer
	tags          []string
}

func (s *LocalStore) newModel(digest oci.Hash, tags []string) (*Model, error) {
	rawManifest, err := os.ReadFile(s.manifestPath(digest))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest, err := oci.ParseManifest(bytes.NewReader(rawManifest))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	configPath, err := s.blobPath(manifest.Config.Digest)
	if err != nil {
		return nil, fmt.Errorf("get config blob path: %w", err)
	}
	rawConfigFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	layers := make([]oci.Layer, len(manifest.Layers))
	for i, ld := range manifest.Layers {
		layerPath, err := s.blobPath(ld.Digest)
		if err != nil {
			return nil, fmt.Errorf("get layer blob path: %w", err)
		}
		layers[i] = &mdpartial.Layer{
			Path:       layerPath,
			Descriptor: ld,
		}
	}

	return &Model{
		rawManifest:   rawManifest,
		manifest:      manifest,
		rawConfigFile: rawConfigFile,
		tags:          tags,
		layers:        layers,
	}, err
}

func (m *Model) Layers() ([]oci.Layer, error) {
	return m.layers, nil
}

func (m *Model) MediaType() (oci.MediaType, error) {
	return m.manifest.MediaType, nil
}

func (m *Model) Size() (int64, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return 0, err
	}
	rawCfg, err := m.RawConfigFile()
	if err != nil {
		return 0, err
	}
	size := int64(len(raw)) + int64(len(rawCfg))
	for _, l := range m.layers {
		s, err := l.Size()
		if err != nil {
			return 0, err
		}
		size += s
	}
	return size, nil
}

func (m *Model) ConfigName() (oci.Hash, error) {
	raw, err := m.RawConfigFile()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *Model) ConfigFile() (*oci.ConfigFile, error) {
	return nil, errors.New("invalid for model")
}

func (m *Model) RawConfigFile() ([]byte, error) {
	return m.rawConfigFile, nil
}

func (m *Model) Digest() (oci.Hash, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return oci.Hash{}, err
	}
	h, _, err := oci.SHA256(bytes.NewReader(raw))
	return h, err
}

func (m *Model) Manifest() (*oci.Manifest, error) {
	return m.manifest, nil
}

func (m *Model) RawManifest() ([]byte, error) {
	return m.rawManifest, nil
}

func (m *Model) LayerByDigest(hash oci.Hash) (oci.Layer, error) {
	for _, l := range m.layers {
		d, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer with digest %s not found", hash)
}

func (m *Model) LayerByDiffID(hash oci.Hash) (oci.Layer, error) {
	return m.LayerByDigest(hash)
}

func (m *Model) GGUFPaths() ([]string, error) {
	return mdpartial.GGUFPaths(m)
}

func (m *Model) MMPROJPath() (string, error) {
	return mdpartial.MMPROJPath(m)
}

func (m *Model) ChatTemplatePath() (string, error) {
	return mdpartial.ChatTemplatePath(m)
}

func (m *Model) SafetensorsPaths() ([]string, error) {
	return mdpartial.SafetensorsPaths(m)
}

func (m *Model) DDUFPaths() ([]string, error) {
	return mdpartial.DDUFPaths(m)
}

func (m *Model) ConfigArchivePath() (string, error) {
	return mdpartial.ConfigArchivePath(m)
}

func (m *Model) Tags() []string {
	return m.tags
}

func (m *Model) ID() (string, error) {
	return mdpartial.ID(m)
}

func (m *Model) Config() (mdtypes.ModelConfig, error) {
	return mdpartial.Config(m)
}

func (m *Model) Descriptor() (mdtypes.Descriptor, error) {
	return mdpartial.Descriptor(m)
}
