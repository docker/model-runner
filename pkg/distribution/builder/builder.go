package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/model-runner/pkg/distribution/format"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/opencontainers/go-digest"
)

// BuildFormat specifies the output artifact format.
type BuildFormat string

const (
	// BuildFormatDocker produces Docker-proprietary format artifacts
	// (application/vnd.docker.ai.* media types). This is the default.
	BuildFormatDocker BuildFormat = "docker"
	// BuildFormatCNCF produces CNCF ModelPack format artifacts
	// (application/vnd.cncf.model.* media types).
	BuildFormatCNCF BuildFormat = "cncf"
)

// BuildOption configures the behavior of FromPath and FromPaths.
type BuildOption func(*buildOptions)

type buildOptions struct {
	created *time.Time
	format  BuildFormat
}

// WithCreated sets a specific creation timestamp for the model artifact.
// When not set, the current time (time.Now()) is used.
// This is useful for producing deterministic OCI digests when the same model
// content should always yield the same artifact regardless of when it was built.
func WithCreated(t time.Time) BuildOption {
	return func(opts *buildOptions) {
		opts.created = &t
	}
}

// WithFormat sets the output artifact format. Defaults to BuildFormatDocker.
func WithFormat(f BuildFormat) BuildOption {
	return func(opts *buildOptions) {
		opts.format = f
	}
}

// Builder builds a model artifact.
type Builder struct {
	model          types.ModelArtifact
	originalLayers []oci.Layer // Snapshot of layers when created from existing model.
	outputFormat   BuildFormat // Output artifact format (docker or cncf).
}

// FromPath returns a *Builder that builds model artifacts from a file path.
// It auto-detects the model format (GGUF or Safetensors) and discovers any shards.
// This is the preferred entry point for creating models from local files.
func FromPath(path string, opts ...BuildOption) (*Builder, error) {
	// Auto-detect format from file extension
	f, err := format.DetectFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("detect format: %w", err)
	}

	// Discover all shards if this is a sharded model
	paths, err := f.DiscoverShards(path)
	if err != nil {
		return nil, fmt.Errorf("discover shards: %w", err)
	}

	// Create model using the format abstraction
	return fromFormat(f, paths, opts...)
}

// FromPaths returns a *Builder that builds model artifacts from multiple file paths.
// All paths must be of the same format. Use this when you already have the list of files.
func FromPaths(paths []string, opts ...BuildOption) (*Builder, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}

	// Detect and verify format from all paths
	f, err := format.DetectFromPaths(paths)
	if err != nil {
		return nil, fmt.Errorf("detect format: %w", err)
	}

	// Create model using the format abstraction
	return fromFormat(f, paths, opts...)
}

// fromFormat creates a Builder using the unified format abstraction.
// This is the internal implementation that creates layers and config.
func fromFormat(f format.Format, paths []string, opts ...BuildOption) (*Builder, error) {
	options := &buildOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Create layers from paths using the Docker media type initially.
	// For CNCF output, media types are remapped below.
	layers := make([]oci.Layer, len(paths))
	diffIDs := make([]oci.Hash, len(paths))

	mediaType := f.MediaType()
	for i, path := range paths {
		layer, err := partial.NewLayer(path, mediaType)
		if err != nil {
			return nil, fmt.Errorf("create layer from %q: %w", path, err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get diffID for %q: %w", path, err)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	// Extract config metadata using format-specific logic.
	config, err := f.ExtractConfig(paths)
	if err != nil {
		return nil, fmt.Errorf("extract config: %w", err)
	}

	// Use the provided creation time, or fall back to current time.
	var created time.Time
	if options.created != nil {
		created = *options.created
	} else {
		created = time.Now()
	}

	if options.format == BuildFormatCNCF {
		return fromFormatCNCF(config, layers, diffIDs, types.Descriptor{Created: &created})
	}

	// Build the Docker-format model (default).
	mdl := &partial.BaseModel{
		ModelConfigFile: types.ConfigFile{
			Config: config,
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: oci.RootFS{
				Type:    "rootfs",
				DiffIDs: diffIDs,
			},
		},
		LayerList: layers,
	}

	return &Builder{
		model:        mdl,
		outputFormat: BuildFormatDocker,
	}, nil
}

// fromFormatCNCF builds a CNCFModel from format-extracted config and layers.
func fromFormatCNCF(
	config types.Config,
	layers []oci.Layer,
	diffIDs []oci.Hash,
	desc types.Descriptor,
) (*Builder, error) {
	// Convert DiffIDs from oci.Hash to digest.Digest.
	cncfDiffIDs := make([]digest.Digest, len(diffIDs))
	for i, d := range diffIDs {
		cncfDiffIDs[i] = digest.Digest(d.String())
	}

	// Remap layer media types to CNCF.
	cncfLayers := make([]oci.Layer, len(layers))
	for i, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			return nil, fmt.Errorf("get layer media type: %w", err)
		}
		fp := layerFilePath(l)
		cncfMT := modelpack.MapLayerMediaType(mt, fp)
		rl, err := newRemappedLayer(l, cncfMT)
		if err != nil {
			return nil, fmt.Errorf("remap layer %d: %w", i, err)
		}
		cncfLayers[i] = rl
	}

	mp := modelpack.DockerConfigToModelPack(config, desc, cncfDiffIDs)
	mdl := &partial.CNCFModel{
		ModelPackConfig: mp,
		LayerList:       cncfLayers,
	}
	return &Builder{
		model:        mdl,
		outputFormat: BuildFormatCNCF,
	}, nil
}

// layerFilePath extracts the filepath annotation from a layer, if present.
func layerFilePath(l oci.Layer) string {
	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}
	if dp, ok := l.(descriptorProvider); ok {
		if fp, ok := dp.GetDescriptor().Annotations[types.AnnotationFilePath]; ok {
			return fp
		}
	}
	return ""
}

// remappedLayer wraps an existing layer and overrides its media type.
// Digest and size are pre-computed at construction time so that
// GetDescriptor never silently swallows errors.
type remappedLayer struct {
	oci.Layer
	newMediaType oci.MediaType
	cachedDigest oci.Hash
	cachedSize   int64
}

// newRemappedLayer creates a remappedLayer, eagerly resolving digest and size
// so that any error (e.g. network failure on a remote layer) surfaces at
// build time rather than producing an invalid OCI descriptor later.
func newRemappedLayer(l oci.Layer, mt oci.MediaType) (*remappedLayer, error) {
	d, err := l.Digest()
	if err != nil {
		return nil, fmt.Errorf("get layer digest: %w", err)
	}
	s, err := l.Size()
	if err != nil {
		return nil, fmt.Errorf("get layer size: %w", err)
	}
	return &remappedLayer{
		Layer:        l,
		newMediaType: mt,
		cachedDigest: d,
		cachedSize:   s,
	}, nil
}

// MediaType returns the remapped media type.
func (r *remappedLayer) MediaType() (oci.MediaType, error) {
	return r.newMediaType, nil
}

// GetDescriptor returns a copy of the underlying descriptor with the
// overridden media type.
func (r *remappedLayer) GetDescriptor() oci.Descriptor {
	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}
	var desc oci.Descriptor
	if dp, ok := r.Layer.(descriptorProvider); ok {
		desc = dp.GetDescriptor()
	} else {
		// Use pre-computed values for layers that are not descriptor
		// providers (e.g. remoteLayer). Errors were already checked in
		// newRemappedLayer.
		desc = oci.Descriptor{Digest: r.cachedDigest, Size: r.cachedSize}
	}
	desc.MediaType = r.newMediaType
	return desc
}

// FromModel returns a *Builder that builds model artifacts from an existing
// model artifact. When WithFormat is provided, the output uses that format.
// When no format is specified, the builder inherits the source model's format
// (auto-detecting CNCF ModelPack via the config). This prevents accidentally
// producing inconsistent artifacts when repackaging a CNCF model without an
// explicit --format flag.
func FromModel(mdl types.ModelArtifact, opts ...BuildOption) (*Builder, error) {
	options := &buildOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Capture original layers for comparison.
	layers, err := mdl.Layers()
	if err != nil {
		return nil, fmt.Errorf("getting model layers: %w", err)
	}

	// Determine output format. If not explicitly set, detect from the model.
	outFmt := options.format
	if outFmt == "" {
		rawCfg, err := mdl.RawConfigFile()
		if err != nil {
			return nil, fmt.Errorf("get raw config for format detection: %w", err)
		}
		if modelpack.IsModelPackConfig(rawCfg) {
			outFmt = BuildFormatCNCF
		} else {
			outFmt = BuildFormatDocker
		}
	}

	if outFmt == BuildFormatCNCF {
		// Convert the source artifact eagerly to CNCF format. This is
		// necessary because mutations (WithLicense, etc.) and lightweight
		// repackaging both operate on the builder state before Build().
		cncfMdl, err := convertToCNCF(mdl)
		if err != nil {
			return nil, fmt.Errorf("convert to cncf format: %w", err)
		}
		return &Builder{
			model:          cncfMdl,
			originalLayers: layers,
			outputFormat:   BuildFormatCNCF,
		}, nil
	}

	return &Builder{
		model:          mdl,
		originalLayers: layers,
		outputFormat:   BuildFormatDocker,
	}, nil
}

// convertToCNCF converts an existing model artifact to a CNCFModel. It remaps
// all layer media types and converts the config to CNCF ModelPack format.
func convertToCNCF(mdl types.ModelArtifact) (*partial.CNCFModel, error) {
	layers, err := mdl.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	// Get the Docker-format config.
	rawCfg, err := mdl.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config: %w", err)
	}

	// Remap layer media types and collect DiffIDs.
	cncfLayers := make([]oci.Layer, len(layers))
	diffIDs := make([]digest.Digest, len(layers))
	for i, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			return nil, fmt.Errorf("get layer media type: %w", err)
		}
		fp := layerFilePath(l)
		cncfMT := modelpack.MapLayerMediaType(mt, fp)
		rl, err := newRemappedLayer(l, cncfMT)
		if err != nil {
			return nil, fmt.Errorf("remap layer %d: %w", i, err)
		}
		cncfLayers[i] = rl

		diffID, err := l.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get layer diffID: %w", err)
		}
		diffIDs[i] = digest.Digest(diffID.String())
	}

	// Build the CNCF config. If the source is already ModelPack format, use
	// it directly (updating the DiffIDs from current layers). Otherwise
	// convert from Docker format.
	var mp modelpack.Model
	if modelpack.IsModelPackConfig(rawCfg) {
		if err := json.Unmarshal(rawCfg, &mp); err != nil {
			return nil, fmt.Errorf("unmarshal modelpack config: %w", err)
		}
		mp.ModelFS.DiffIDs = diffIDs
	} else {
		var cf types.ConfigFile
		if err := json.Unmarshal(rawCfg, &cf); err != nil {
			return nil, fmt.Errorf("unmarshal docker config: %w", err)
		}
		mp = modelpack.DockerConfigToModelPack(cf.Config, cf.Descriptor, diffIDs)
	}

	return &partial.CNCFModel{
		ModelPackConfig: mp,
		LayerList:       cncfLayers,
	}, nil
}

// resolveLayerMediaType returns the appropriate media type for an additional
// layer based on the builder's output format. For CNCF format, Docker media
// types are remapped to their CNCF equivalents.
func (b *Builder) resolveLayerMediaType(dockerMT oci.MediaType) oci.MediaType {
	if b.outputFormat == BuildFormatCNCF {
		return modelpack.MapLayerMediaType(dockerMT, "")
	}
	return dockerMT
}

// WithLicense adds a license file to the artifact.
func (b *Builder) WithLicense(path string) (*Builder, error) {
	mt := b.resolveLayerMediaType(types.MediaTypeLicense)
	licenseLayer, err := partial.NewLayer(path, mt)
	if err != nil {
		return nil, fmt.Errorf("license layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, licenseLayer),
		originalLayers: b.originalLayers,
		outputFormat:   b.outputFormat,
	}, nil
}

// WithContextSize sets the context size for the model artifact.
// Returns an error when the output format is CNCF (context size is not
// defined in the CNCF ModelPack specification).
func (b *Builder) WithContextSize(size int32) (*Builder, error) {
	if b.outputFormat == BuildFormatCNCF {
		return nil, fmt.Errorf(
			"--context-size is not supported with --format cncf: " +
				"the CNCF ModelPack specification does not define a context " +
				"size field",
		)
	}
	return &Builder{
		model:          mutate.ContextSize(b.model, size),
		originalLayers: b.originalLayers,
		outputFormat:   b.outputFormat,
	}, nil
}

// WithMultimodalProjector adds a multimodal projector file to the artifact.
func (b *Builder) WithMultimodalProjector(path string) (*Builder, error) {
	mt := b.resolveLayerMediaType(types.MediaTypeMultimodalProjector)
	mmprojLayer, err := partial.NewLayer(path, mt)
	if err != nil {
		return nil, fmt.Errorf("mmproj layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, mmprojLayer),
		originalLayers: b.originalLayers,
		outputFormat:   b.outputFormat,
	}, nil
}

// WithChatTemplateFile adds a Jinja chat template file to the artifact,
// taking precedence over any template embedded in the GGUF file.
func (b *Builder) WithChatTemplateFile(path string) (*Builder, error) {
	mt := b.resolveLayerMediaType(types.MediaTypeChatTemplate)
	templateLayer, err := partial.NewLayer(path, mt)
	if err != nil {
		return nil, fmt.Errorf("chat template layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, templateLayer),
		originalLayers: b.originalLayers,
		outputFormat:   b.outputFormat,
	}, nil
}

// Target represents a build target
type Target interface {
	Write(context.Context, types.ModelArtifact, io.Writer) error
}

// Model returns the underlying model artifact
func (b *Builder) Model() types.ModelArtifact {
	return b.model
}

// Build finalizes the artifact and writes it to the given target, reporting progress to the given writer
func (b *Builder) Build(ctx context.Context, target Target, pw io.Writer) error {
	return target.Write(ctx, b.model, pw)
}

// HasOnlyConfigChanges returns true if the builder was created from an existing model
// and only configuration changes were made (no layers added or removed).
// This is useful for determining if lightweight repackaging optimizations can be used.
func (b *Builder) HasOnlyConfigChanges() bool {
	// If not created from an existing model, return false
	if b.originalLayers == nil {
		return false
	}

	// Get current layers
	currentLayers, err := b.model.Layers()
	if err != nil {
		return false
	}

	// If layer count changed, files were added or removed
	if len(currentLayers) != len(b.originalLayers) {
		return false
	}

	// Verify layer digests match to ensure no layer content changed
	for i, origLayer := range b.originalLayers {
		origDigest, err := origLayer.Digest()
		if err != nil {
			return false
		}
		currDigest, err := currentLayers[i].Digest()
		if err != nil {
			return false
		}
		if origDigest != currDigest {
			return false
		}
	}

	return true
}
