package builder

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/model-runner/pkg/distribution/format"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/packaging"
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

// FromDirectoryResult contains the result of creating a builder from a directory,
// including a cleanup function that must be called when done.
type FromDirectoryResult struct {
	Builder *Builder
	Cleanup func()
}

// FromDirectory creates a Builder from a HuggingFace-style model directory.
// It recursively scans the directory, packaging large weight files as separate OCI layers
// with their relative paths preserved in annotations, while grouping small config files
// into a tar archive.
//
// The returned cleanup function MUST be called after building to remove temporary files.
//
// Example usage:
//
//	result, err := builder.FromDirectory("/path/to/model")
//	if err != nil {
//	    return err
//	}
//	defer result.Cleanup()
//	err = result.Builder.Build(ctx, target, progressWriter)
func FromDirectory(dirPath string) (*FromDirectoryResult, error) {
	return FromDirectoryWithOptions(dirPath, DefaultDirectoryOptions())
}

// DirectoryOptions configures how a directory is packaged into a model artifact
type DirectoryOptions struct {
	// Format specifies the expected model format. If empty, it will be auto-detected.
	Format string
}

// DefaultDirectoryOptions returns the default options for directory packaging
func DefaultDirectoryOptions() DirectoryOptions {
	return DirectoryOptions{}
}

// FromDirectoryWithOptions creates a Builder from a model directory with custom options.
// See FromDirectory for usage details.
func FromDirectoryWithOptions(dirPath string, opts DirectoryOptions) (*FromDirectoryResult, error) {
	// Import packaging here to avoid circular dependencies
	// This is done inline to keep the import minimal
	result, err := packDirectory(dirPath)
	if err != nil {
		return nil, fmt.Errorf("package directory: %w", err)
	}

	// Cleanup function to remove temporary files
	cleanup := func() {
		if result.ConfigTarPath != "" {
			removeFile(result.ConfigTarPath)
		}
	}

	// Determine format from result or options
	modelFormat := result.Format
	if opts.Format != "" {
		modelFormat = opts.Format
	}

	// Create layers for weight files with relative path annotations
	layers := make([]oci.Layer, 0, len(result.WeightFiles))
	diffIDs := make([]oci.Hash, 0, len(result.WeightFiles))

	// Determine media type based on format
	mediaType := mediaTypeForFormat(modelFormat)

	for _, wf := range result.WeightFiles {
		// Use NewLayerWithRelativePath to preserve the directory structure
		layer, err := partial.NewLayerWithRelativePath(wf.AbsPath, wf.RelPath, mediaType)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("create layer for %q: %w", wf.RelPath, err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("get diffID for %q: %w", wf.RelPath, err)
		}
		layers = append(layers, layer)
		diffIDs = append(diffIDs, diffID)
	}

	// Extract config from the first weight file for format-specific metadata
	var config types.Config
	if len(result.WeightFiles) > 0 {
		f, err := format.DetectFromPath(result.WeightFiles[0].AbsPath)
		if err == nil {
			paths := make([]string, len(result.WeightFiles))
			for i, wf := range result.WeightFiles {
				paths[i] = wf.AbsPath
			}
			config, _ = f.ExtractConfig(paths)
		}
	}
	if config.Format == "" {
		config.Format = types.Format(modelFormat)
	}

	// Build the base model with v0.2 config version for nested directory support
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
		// Use v0.2 config version to indicate this model has filepath annotations
		// for nested directory support
		ConfigMediaType: types.MediaTypeModelConfigV02,
	}

	builder := &Builder{
		model: mdl,
	}

	// Add config tar as a DirTar layer if it exists
	if result.ConfigTarPath != "" {
		builder, err = builder.WithDirTar(result.ConfigTarPath)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("add config archive: %w", err)
		}
	}

	return &FromDirectoryResult{
		Builder: builder,
		Cleanup: cleanup,
	}, nil
}

// packDirectory wraps the packaging.PackageFromDirectoryRecursive function
func packDirectory(dirPath string) (*directoryPackageResult, error) {
	pkgResult, err := packaging.PackageFromDirectoryRecursive(dirPath)
	if err != nil {
		return nil, err
	}

	// Convert to our internal type
	result := &directoryPackageResult{
		ConfigTarPath: pkgResult.ConfigTarPath,
		Format:        pkgResult.Format,
		WeightFiles:   make([]weightFileInfo, len(pkgResult.WeightFiles)),
	}
	for i, wf := range pkgResult.WeightFiles {
		result.WeightFiles[i] = weightFileInfo{
			AbsPath: wf.AbsPath,
			RelPath: wf.RelPath,
			Size:    wf.Size,
		}
	}
	return result, nil
}

// directoryPackageResult mirrors packaging.PackageResult to avoid import in type signature
type directoryPackageResult struct {
	WeightFiles   []weightFileInfo
	ConfigTarPath string
	Format        string
}

type weightFileInfo struct {
	AbsPath string
	RelPath string
	Size    int64
}

// mediaTypeForFormat returns the OCI media type for a given format string
func mediaTypeForFormat(formatStr string) oci.MediaType {
	switch formatStr {
	case "safetensors":
		return types.MediaTypeSafetensors
	case "gguf":
		return types.MediaTypeGGUF
	case "dduf":
		return types.MediaTypeDDUF
	default:
		return types.MediaTypeSafetensors // default
	}
}

// removeFile is a helper to remove a file, ignoring errors
func removeFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
