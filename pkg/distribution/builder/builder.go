package builder

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/model-runner/pkg/distribution/format"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// Builder builds a model artifact
type Builder struct {
	model          types.ModelArtifact
	originalLayers []oci.Layer // Snapshot of layers when created from existing model
}

// FromPath returns a *Builder that builds model artifacts from a file path.
// It auto-detects the model format (GGUF or Safetensors) and discovers any shards.
// This is the preferred entry point for creating models from local files.
func FromPath(path string) (*Builder, error) {
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
	return fromFormat(f, paths)
}

// FromPaths returns a *Builder that builds model artifacts from multiple file paths.
// All paths must be of the same format. Use this when you already have the list of files.
func FromPaths(paths []string) (*Builder, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}

	// Detect and verify format from all paths
	f, err := format.DetectFromPaths(paths)
	if err != nil {
		return nil, fmt.Errorf("detect format: %w", err)
	}

	// Create model using the format abstraction
	return fromFormat(f, paths)
}

// fromFormat creates a Builder using the unified format abstraction.
// This is the internal implementation that creates layers and config.
func fromFormat(f format.Format, paths []string) (*Builder, error) {
	// Create layers from paths
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

	// Extract config metadata using format-specific logic
	config, err := f.ExtractConfig(paths)
	if err != nil {
		return nil, fmt.Errorf("extract config: %w", err)
	}

	// Build the model
	created := time.Now()
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
		model: mdl,
	}, nil
}

// FromModel returns a *Builder that builds model artifacts from an existing model artifact
func FromModel(mdl types.ModelArtifact) (*Builder, error) {
	// Capture original layers for comparison
	layers, err := mdl.Layers()
	if err != nil {
		return nil, fmt.Errorf("getting model layers: %w", err)
	}
	return &Builder{
		model:          mdl,
		originalLayers: layers,
	}, nil
}

// WithLicense adds a license file to the artifact
func (b *Builder) WithLicense(path string) (*Builder, error) {
	licenseLayer, err := partial.NewLayer(path, types.MediaTypeLicense)
	if err != nil {
		return nil, fmt.Errorf("license layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, licenseLayer),
		originalLayers: b.originalLayers,
	}, nil
}

func (b *Builder) WithContextSize(size int32) *Builder {
	return &Builder{
		model:          mutate.ContextSize(b.model, size),
		originalLayers: b.originalLayers,
	}
}

// WithMultimodalProjector adds a Multimodal projector file to the artifact
func (b *Builder) WithMultimodalProjector(path string) (*Builder, error) {
	mmprojLayer, err := partial.NewLayer(path, types.MediaTypeMultimodalProjector)
	if err != nil {
		return nil, fmt.Errorf("mmproj layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, mmprojLayer),
		originalLayers: b.originalLayers,
	}, nil
}

// WithChatTemplateFile adds a Jinja chat template file to the artifact which takes precedence over template from GGUF.
func (b *Builder) WithChatTemplateFile(path string) (*Builder, error) {
	templateLayer, err := partial.NewLayer(path, types.MediaTypeChatTemplate)
	if err != nil {
		return nil, fmt.Errorf("chat template layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, templateLayer),
		originalLayers: b.originalLayers,
	}, nil
}

// WithConfigArchive adds a config archive (tar) file to the artifact
func (b *Builder) WithConfigArchive(path string) (*Builder, error) {
	// Check if config archive already exists
	layers, err := b.model.Layers()
	if err != nil {
		return nil, fmt.Errorf("get model layers: %w", err)
	}

	for _, layer := range layers {
		mediaType, mediaTypeErr := layer.MediaType()
		if mediaTypeErr == nil && mediaType == types.MediaTypeVLLMConfigArchive {
			return nil, fmt.Errorf("model already has a config archive layer")
		}
	}

	configLayer, err := partial.NewLayer(path, types.MediaTypeVLLMConfigArchive)
	if err != nil {
		return nil, fmt.Errorf("config archive layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, configLayer),
		originalLayers: b.originalLayers,
	}, nil
}

// WithDirTar adds a directory tar archive to the artifact.
// Multiple directory tar archives can be added by calling this method multiple times.
func (b *Builder) WithDirTar(path string) (*Builder, error) {
	dirTarLayer, err := partial.NewLayer(path, types.MediaTypeDirTar)
	if err != nil {
		return nil, fmt.Errorf("dir tar layer from %q: %w", path, err)
	}
	return &Builder{
		model:          mutate.AppendLayers(b.model, dirTarLayer),
		originalLayers: b.originalLayers,
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
