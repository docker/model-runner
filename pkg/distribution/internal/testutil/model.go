package testutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/opencontainers/go-digest"
)

// BuildModelFromPath constructs a model artifact from a file path and fails the test on error.
func BuildModelFromPath(t *testing.T, path string) types.ModelArtifact {
	t.Helper()

	b, err := builder.FromPath(path)
	if err != nil {
		t.Fatalf("Failed to create model from path %q: %v", path, err)
	}
	return b.Model()
}

// LayerSpec describes a test layer to create from a local file path.
type LayerSpec struct {
	Path         string
	RelativePath string
	MediaType    oci.MediaType
}

// Layer creates a layer spec with the file basename set as the unpacked relative path.
func Layer(path string, mediaType oci.MediaType) LayerSpec {
	return LayerSpec{
		Path:         path,
		RelativePath: filepath.Base(path),
		MediaType:    mediaType,
	}
}

// Artifact is a generic, configurable OCI model artifact for testing.
// It accepts raw config bytes, a config media type, and an arbitrary set of layers,
// allowing tests to construct Docker model-spec, ModelPack, or any custom OCI artifact
// without re-implementing the ModelArtifact interface in every test file.
type Artifact struct {
	rawConfig       []byte
	configMediaType oci.MediaType
	layers          []oci.Layer
}

var _ types.ModelArtifact = (*Artifact)(nil)

// NewArtifact creates a new generic test artifact with the given raw config bytes,
// config media type, and layers.
func NewArtifact(rawConfig []byte, configMediaType oci.MediaType, layers ...oci.Layer) *Artifact {
	return &Artifact{
		rawConfig:       rawConfig,
		configMediaType: configMediaType,
		layers:          layers,
	}
}

// NewDockerArtifact creates a Docker-format test artifact with the default config media type.
func NewDockerArtifact(t *testing.T, cfg types.Config, layers ...LayerSpec) *Artifact {
	t.Helper()
	return NewDockerArtifactWithConfigMediaType(t, cfg, types.MediaTypeModelConfigV01, layers...)
}

// NewDockerArtifactWithConfigMediaType creates a Docker-format test artifact with the given config media type.
func NewDockerArtifactWithConfigMediaType(
	t *testing.T,
	cfg types.Config,
	configMediaType oci.MediaType,
	layers ...LayerSpec,
) *Artifact {
	t.Helper()

	builtLayers := buildLayers(t, layers...)
	rawConfig, err := json.Marshal(types.ConfigFile{
		Config: cfg,
		RootFS: oci.RootFS{
			Type:    "layers",
			DiffIDs: dockerDiffIDs(t, builtLayers),
		},
	})
	if err != nil {
		t.Fatalf("Failed to marshal Docker test config: %v", err)
	}

	return NewArtifact(rawConfig, configMediaType, builtLayers...)
}

// NewGGUFArtifact creates a Docker-format GGUF test artifact.
func NewGGUFArtifact(t *testing.T, modelPath string, extraLayers ...LayerSpec) *Artifact {
	t.Helper()

	layers := append([]LayerSpec{Layer(modelPath, types.MediaTypeGGUF)}, extraLayers...)
	return NewDockerArtifact(t, types.Config{Format: types.FormatGGUF}, layers...)
}

// NewGGUFArtifactWithConfigMediaType creates a GGUF test artifact with a custom Docker config media type.
func NewGGUFArtifactWithConfigMediaType(
	t *testing.T,
	modelPath string,
	configMediaType oci.MediaType,
	extraLayers ...LayerSpec,
) *Artifact {
	t.Helper()

	layers := append([]LayerSpec{Layer(modelPath, types.MediaTypeGGUF)}, extraLayers...)
	return NewDockerArtifactWithConfigMediaType(t, types.Config{Format: types.FormatGGUF}, configMediaType, layers...)
}

// NewSafetensorsArtifact creates a Docker-format safetensors test artifact.
func NewSafetensorsArtifact(t *testing.T, modelPath string, extraLayers ...LayerSpec) *Artifact {
	t.Helper()

	layers := append([]LayerSpec{Layer(modelPath, types.MediaTypeSafetensors)}, extraLayers...)
	return NewDockerArtifact(t, types.Config{Format: types.FormatSafetensors}, layers...)
}

// NewModelPackArtifact creates a ModelPack-format test artifact and populates ModelFS DiffIDs from the layers.
func NewModelPackArtifact(t *testing.T, model modelpack.Model, layers ...LayerSpec) *Artifact {
	t.Helper()

	builtLayers := buildLayers(t, layers...)
	model.ModelFS = modelpack.ModelFS{
		Type:    "layers",
		DiffIDs: modelPackDiffIDs(t, builtLayers),
	}

	rawConfig, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("Failed to marshal ModelPack test config: %v", err)
	}

	return NewArtifact(rawConfig, modelpack.MediaTypeModelConfigV1, builtLayers...)
}

// GetConfigMediaType implements partial.WithConfigMediaType so that ManifestForLayers
// uses the correct config media type when building the OCI manifest.
func (a *Artifact) GetConfigMediaType() oci.MediaType {
	return a.configMediaType
}

// ID implements types.ModelArtifact.
func (a *Artifact) ID() (string, error) {
	return partial.ID(a)
}

// Config implements types.ModelArtifact.
func (a *Artifact) Config() (types.ModelConfig, error) {
	return partial.Config(a)
}

// Descriptor implements types.ModelArtifact.
func (a *Artifact) Descriptor() (types.Descriptor, error) {
	raw, err := a.RawConfigFile()
	if err != nil {
		return types.Descriptor{}, err
	}
	return descriptorFromRawConfig(raw)
}

// RawConfigFile implements partial.WithRawConfigFile.
func (a *Artifact) RawConfigFile() ([]byte, error) {
	return a.rawConfig, nil
}

// Layers implements oci.Image.
func (a *Artifact) Layers() ([]oci.Layer, error) {
	return a.layers, nil
}

// MediaType implements oci.Image.
func (a *Artifact) MediaType() (oci.MediaType, error) {
	m, err := a.Manifest()
	if err != nil {
		return "", err
	}
	return m.MediaType, nil
}

// Size implements oci.Image.
func (a *Artifact) Size() (int64, error) {
	rawManifest, err := a.RawManifest()
	if err != nil {
		return 0, err
	}
	size := int64(len(rawManifest) + len(a.rawConfig))
	for _, l := range a.layers {
		ls, err := l.Size()
		if err != nil {
			return 0, err
		}
		size += ls
	}
	return size, nil
}

// ConfigName implements oci.Image.
func (a *Artifact) ConfigName() (oci.Hash, error) {
	hash, _, err := oci.SHA256(bytes.NewReader(a.rawConfig))
	return hash, err
}

// ConfigFile implements oci.Image. Model artifacts do not have a standard OCI config file.
func (a *Artifact) ConfigFile() (*oci.ConfigFile, error) {
	return nil, errors.New("not supported for model artifacts")
}

// Digest implements oci.Image.
func (a *Artifact) Digest() (oci.Hash, error) {
	raw, err := a.RawManifest()
	if err != nil {
		return oci.Hash{}, err
	}
	hash, _, err := oci.SHA256(bytes.NewReader(raw))
	return hash, err
}

// Manifest implements oci.Image.
func (a *Artifact) Manifest() (*oci.Manifest, error) {
	return partial.ManifestForLayers(a)
}

// RawManifest implements oci.Image.
func (a *Artifact) RawManifest() ([]byte, error) {
	m, err := a.Manifest()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// LayerByDigest implements oci.Image.
func (a *Artifact) LayerByDigest(hash oci.Hash) (oci.Layer, error) {
	for _, l := range a.layers {
		d, err := l.Digest()
		if err != nil {
			return nil, err
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer with digest %s not found", hash)
}

// LayerByDiffID implements oci.Image.
func (a *Artifact) LayerByDiffID(hash oci.Hash) (oci.Layer, error) {
	for _, l := range a.layers {
		d, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer with diffID %s not found", hash)
}

// NewStaticLayer creates an in-memory layer for tests that do not need a backing file path.
func NewStaticLayer(content []byte, mediaType oci.MediaType) oci.Layer {
	hash, _, err := oci.SHA256(bytes.NewReader(content))
	if err != nil {
		panic(fmt.Sprintf("compute static layer hash: %v", err))
	}
	return &staticLayer{
		content:   content,
		mediaType: mediaType,
		hash:      hash,
	}
}

// WithRawConfigError returns a wrapper that fails when RawConfigFile is requested.
func WithRawConfigError(base types.ModelArtifact, err error) types.ModelArtifact {
	return rawConfigErrorArtifact{
		ModelArtifact: base,
		err:           err,
	}
}

// WithLayersError returns a wrapper that fails when Layers is requested.
func WithLayersError(base types.ModelArtifact, err error) types.ModelArtifact {
	return layersErrorArtifact{
		ModelArtifact: base,
		err:           err,
	}
}

type rawConfigErrorArtifact struct {
	types.ModelArtifact
	err error
}

func (a rawConfigErrorArtifact) RawConfigFile() ([]byte, error) {
	return nil, a.err
}

func (a rawConfigErrorArtifact) Config() (types.ModelConfig, error) {
	return partial.Config(a)
}

func (a rawConfigErrorArtifact) Descriptor() (types.Descriptor, error) {
	return partial.Descriptor(a)
}

type layersErrorArtifact struct {
	types.ModelArtifact
	err error
}

func (a layersErrorArtifact) Layers() ([]oci.Layer, error) {
	return nil, a.err
}

func (a layersErrorArtifact) LayerByDigest(oci.Hash) (oci.Layer, error) {
	return nil, a.err
}

func (a layersErrorArtifact) LayerByDiffID(oci.Hash) (oci.Layer, error) {
	return nil, a.err
}

type staticLayer struct {
	content   []byte
	mediaType oci.MediaType
	hash      oci.Hash
}

func (l *staticLayer) Digest() (oci.Hash, error)         { return l.hash, nil }
func (l *staticLayer) DiffID() (oci.Hash, error)         { return l.hash, nil }
func (l *staticLayer) Size() (int64, error)              { return int64(len(l.content)), nil }
func (l *staticLayer) MediaType() (oci.MediaType, error) { return l.mediaType, nil }
func (l *staticLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *staticLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func buildLayers(t *testing.T, specs ...LayerSpec) []oci.Layer {
	t.Helper()

	layers := make([]oci.Layer, 0, len(specs))
	for _, spec := range specs {
		var (
			layer *partial.Layer
			err   error
		)

		if spec.RelativePath == "" {
			layer, err = partial.NewLayer(spec.Path, spec.MediaType)
		} else {
			layer, err = partial.NewLayerWithRelativePath(spec.Path, spec.RelativePath, spec.MediaType)
		}
		if err != nil {
			t.Fatalf("Failed to create test layer %q: %v", spec.Path, err)
		}

		layers = append(layers, layer)
	}

	return layers
}

func dockerDiffIDs(t *testing.T, layers []oci.Layer) []oci.Hash {
	t.Helper()

	diffIDs := make([]oci.Hash, 0, len(layers))
	for _, layer := range layers {
		diffID, err := layer.DiffID()
		if err != nil {
			t.Fatalf("Failed to get test layer diffID: %v", err)
		}
		diffIDs = append(diffIDs, diffID)
	}

	return diffIDs
}

func modelPackDiffIDs(t *testing.T, layers []oci.Layer) []digest.Digest {
	t.Helper()

	diffIDs := make([]digest.Digest, 0, len(layers))
	for _, layer := range layers {
		diffID, err := layer.DiffID()
		if err != nil {
			t.Fatalf("Failed to get test layer diffID: %v", err)
		}
		diffIDs = append(diffIDs, digest.Digest(diffID.String()))
	}

	return diffIDs
}

func descriptorFromRawConfig(raw []byte) (types.Descriptor, error) {
	if modelpack.IsModelPackConfig(raw) {
		var mp modelpack.Model
		if err := json.Unmarshal(raw, &mp); err != nil {
			return types.Descriptor{}, fmt.Errorf("unmarshal modelpack config: %w", err)
		}
		return types.Descriptor{Created: mp.Descriptor.CreatedAt}, nil
	}

	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return types.Descriptor{}, fmt.Errorf("unmarshal config: %w", err)
	}
	return cf.Descriptor, nil
}
