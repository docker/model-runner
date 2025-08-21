package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"

	mdpartial "github.com/docker/model-distribution/internal/partial"
	mdtypes "github.com/docker/model-distribution/types"
)

var _ v1.Image = &Model{}

type Model struct {
	rawManifest   []byte
	manifest      *v1.Manifest
	rawConfigFile []byte
	layers        []v1.Layer
	tags          []string
}

func (s *LocalStore) newModel(digest v1.Hash, tags []string) (*Model, error) {
	rawManifest, err := os.ReadFile(s.manifestPath(digest))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest, err := v1.ParseManifest(bytes.NewReader(rawManifest))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	rawConfigFile, err := os.ReadFile(s.blobPath(manifest.Config.Digest))
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	layers := make([]v1.Layer, len(manifest.Layers))
	for i, ld := range manifest.Layers {
		layers[i] = &mdpartial.Layer{
			Path:       s.blobPath(ld.Digest),
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

func (m *Model) Layers() ([]v1.Layer, error) {
	return m.layers, nil
}

func (m *Model) MediaType() (types.MediaType, error) {
	return m.manifest.MediaType, nil
}

func (m *Model) Size() (int64, error) {
	return partial.Size(m)
}

func (m *Model) ConfigName() (v1.Hash, error) {
	return partial.ConfigName(m)
}

func (m *Model) ConfigFile() (*v1.ConfigFile, error) {
	return nil, errors.New("invalid for model")
}

func (m *Model) RawConfigFile() ([]byte, error) {
	return m.rawConfigFile, nil
}

func (m *Model) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

func (m *Model) Manifest() (*v1.Manifest, error) {
	return partial.Manifest(m)
}

func (m *Model) RawManifest() ([]byte, error) {
	return m.rawManifest, nil
}

func (m *Model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
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

func (m *Model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	return m.LayerByDigest(hash)
}

func (m *Model) GGUFPath() (string, error) {
	return mdpartial.GGUFPath(m)
}

func (m *Model) MMPROJPath() (string, error) {
	return mdpartial.MMPROJPath(m)
}

func (m *Model) Tags() []string {
	return m.tags
}

func (m *Model) ID() (string, error) {
	return mdpartial.ID(m)
}

func (m *Model) Config() (mdtypes.Config, error) {
	return mdpartial.Config(m)
}

func (m *Model) Descriptor() (mdtypes.Descriptor, error) {
	return mdpartial.Descriptor(m)
}
