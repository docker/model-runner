package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Helpers for computing image metadata from partial information.

// WithRawManifest represents types that can provide raw manifest bytes.
type WithRawManifest interface {
	RawManifest() ([]byte, error)
}

// WithManifest represents types that can provide a manifest.
type WithManifest interface {
	Manifest() (*Manifest, error)
}

// WithRawConfigFile represents types that can provide raw config file bytes.
type WithRawConfigFile interface {
	RawConfigFile() ([]byte, error)
}

// WithConfigFile represents types that can provide a config file.
type WithConfigFile interface {
	ConfigFile() (*ConfigFile, error)
}

// WithLayers represents types that can provide layers.
type WithLayers interface {
	Layers() ([]Layer, error)
}

// Digest computes the digest of an image from its raw manifest.
func Digest(i WithRawManifest) (Hash, error) {
	raw, err := i.RawManifest()
	if err != nil {
		return Hash{}, err
	}
	h, _, err := SHA256(bytes.NewReader(raw))
	return h, err
}

// Size computes the total size of an image (manifest + config + layers).
func Size(i interface {
	WithRawManifest
	WithRawConfigFile
	WithLayers
}) (int64, error) {
	rawManifest, err := i.RawManifest()
	if err != nil {
		return 0, err
	}

	rawConfig, err := i.RawConfigFile()
	if err != nil {
		return 0, err
	}

	layers, err := i.Layers()
	if err != nil {
		return 0, err
	}

	size := int64(len(rawManifest)) + int64(len(rawConfig))
	for _, l := range layers {
		s, err := l.Size()
		if err != nil {
			return 0, err
		}
		size += s
	}
	return size, nil
}

// ConfigName computes the config name (digest of config file) from raw config bytes.
func ConfigName(i WithRawConfigFile) (Hash, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return Hash{}, err
	}
	h, _, err := SHA256(bytes.NewReader(raw))
	return h, err
}

// RawManifest computes the raw manifest bytes from a manifest object.
func RawManifest(i WithManifest) ([]byte, error) {
	m, err := i.Manifest()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// ConfigLayer returns a layer representing the config blob.
func ConfigLayer(i WithRawConfigFile) (Layer, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, err
	}
	h, _, err := SHA256(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return &configLayer{
		content: raw,
		hash:    h,
	}, nil
}

// configLayer is a Layer implementation for config blobs.
type configLayer struct {
	content []byte
	hash    Hash
}

func (c *configLayer) Digest() (Hash, error) {
	return c.hash, nil
}

func (c *configLayer) DiffID() (Hash, error) {
	return c.hash, nil
}

func (c *configLayer) Compressed() (io.ReadCloser, error) {
	return &bytesReadCloser{bytes.NewReader(c.content)}, nil
}

func (c *configLayer) Uncompressed() (io.ReadCloser, error) {
	return c.Compressed()
}

func (c *configLayer) Size() (int64, error) {
	return int64(len(c.content)), nil
}

func (c *configLayer) MediaType() (MediaType, error) {
	return OCIConfigJSON, nil
}

// bytesReadCloser wraps a bytes.Reader with a Close method.
type bytesReadCloser struct {
	*bytes.Reader
}

func (b *bytesReadCloser) Close() error {
	return nil
}

// LayerDescriptor computes a descriptor from a layer.
func LayerDescriptor(l Layer) (*Descriptor, error) {
	mt, err := l.MediaType()
	if err != nil {
		return nil, fmt.Errorf("getting media type: %w", err)
	}

	size, err := l.Size()
	if err != nil {
		return nil, fmt.Errorf("getting size: %w", err)
	}

	digest, err := l.Digest()
	if err != nil {
		return nil, fmt.Errorf("getting digest: %w", err)
	}

	return &Descriptor{
		MediaType: mt,
		Size:      size,
		Digest:    digest,
	}, nil
}
