package oci

import (
	"bytes"
	"encoding/json"
	"io"
)

// Descriptor describes a blob in a registry.
type Descriptor struct {
	MediaType    MediaType         `json:"mediaType"`
	Size         int64             `json:"size"`
	Digest       Hash              `json:"digest"`
	Data         []byte            `json:"data,omitempty"`
	URLs         []string          `json:"urls,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Platform     *Platform         `json:"platform,omitempty"`
	ArtifactType string            `json:"artifactType,omitempty"`
}

// Platform represents the target platform of an image.
type Platform struct {
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os.version,omitempty"`
	OSFeatures   []string `json:"os.features,omitempty"`
	Variant      string   `json:"variant,omitempty"`
}

// Manifest represents an OCI image manifest.
type Manifest struct {
	SchemaVersion int64             `json:"schemaVersion"`
	MediaType     MediaType         `json:"mediaType,omitempty"`
	Config        Descriptor        `json:"config"`
	Layers        []Descriptor      `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Subject       *Descriptor       `json:"subject,omitempty"`
}

// IndexManifest represents an OCI image index (multi-platform manifest list).
type IndexManifest struct {
	SchemaVersion int64             `json:"schemaVersion"`
	MediaType     MediaType         `json:"mediaType,omitempty"`
	Manifests     []Descriptor      `json:"manifests"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Subject       *Descriptor       `json:"subject,omitempty"`
}

// ParseManifest parses the io.Reader's contents into a Manifest.
func ParseManifest(r io.Reader) (*Manifest, error) {
	m := Manifest{}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ParseIndexManifest parses the io.Reader's contents into an IndexManifest.
func ParseIndexManifest(r io.Reader) (*IndexManifest, error) {
	im := IndexManifest{}
	if err := json.NewDecoder(r).Decode(&im); err != nil {
		return nil, err
	}
	return &im, nil
}

// RawManifest returns the serialized bytes of the Manifest.
func (m *Manifest) RawManifest() ([]byte, error) {
	return json.Marshal(m)
}

// ComputeDigest computes the digest of the manifest.
func (m *Manifest) ComputeDigest() (Hash, error) {
	raw, err := m.RawManifest()
	if err != nil {
		return Hash{}, err
	}
	h, _, err := SHA256(bytes.NewReader(raw))
	return h, err
}
