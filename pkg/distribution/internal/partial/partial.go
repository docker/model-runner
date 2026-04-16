package partial

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/files"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

type WithRawConfigFile interface {
	// RawConfigFile returns the serialized bytes of this model's config file.
	RawConfigFile() ([]byte, error)
}

// Config returns the model configuration. Returns *types.Config for Docker format
// or *modelpack.Model for ModelPack format, without any conversion.
func Config(i WithRawConfigFile) (types.ModelConfig, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}

	// ModelPack format: parse directly into modelpack.Model without conversion
	if modelpack.IsModelPackConfig(raw) {
		var mp modelpack.Model
		if err := json.Unmarshal(raw, &mp); err != nil {
			return nil, fmt.Errorf("unmarshal modelpack config: %w", err)
		}
		return &mp, nil
	}

	// Docker format: parse into types.Config
	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cf.Config, nil
}

// ConfigFile returns the full Docker format config file (only for Docker format models).
func ConfigFile(i WithRawConfigFile) (*types.ConfigFile, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}

	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cf, nil
}

// Descriptor returns the types.Descriptor for the model.
func Descriptor(i WithRawConfigFile) (types.Descriptor, error) {
	cf, err := ConfigFile(i)
	if err != nil {
		return types.Descriptor{}, fmt.Errorf("config file: %w", err)
	}
	return cf.Descriptor, nil
}

// WithRawManifest defines the subset of types.Model used by these helper methods
type WithRawManifest interface {
	// RawManifest returns the serialized bytes of this model's manifest file.
	RawManifest() ([]byte, error)
}

func ID(i WithRawManifest) (string, error) {
	raw, err := i.RawManifest()
	if err != nil {
		return "", fmt.Errorf("get raw manifest: %w", err)
	}
	digest, _, err := oci.SHA256(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("compute digest: %w", err)
	}
	return digest.String(), nil
}

type WithLayers interface {
	WithRawConfigFile
	Layers() ([]oci.Layer, error)
}

func GGUFPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeGGUF, getModelFormat(i))
}

func MMPROJPath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeMultimodalProjector, "")
	if err != nil {
		return "", fmt.Errorf("get mmproj layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeMultimodalProjector)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeMultimodalProjector)
	}
	return paths[0], err
}

func ChatTemplatePath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeChatTemplate, "")
	if err != nil {
		return "", fmt.Errorf("get chat template layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeChatTemplate)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeChatTemplate)
	}
	return paths[0], err
}

func SafetensorsPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeSafetensors, getModelFormat(i))
}

func DDUFPaths(i WithLayers) ([]string, error) {
	return layerPathsByMediaType(i, types.MediaTypeDDUF, getModelFormat(i))
}

func ConfigArchivePath(i WithLayers) (string, error) {
	paths, err := layerPathsByMediaType(i, types.MediaTypeVLLMConfigArchive, "")
	if err != nil {
		return "", fmt.Errorf("get config archive layer paths: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("model does not contain any layer of type %q", types.MediaTypeVLLMConfigArchive)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("found %d files of type %q, expected exactly 1",
			len(paths), types.MediaTypeVLLMConfigArchive)
	}
	return paths[0], err
}

// getModelFormat reads the model config and returns the format string (e.g.,
// "gguf", "safetensors"). Used to resolve format-agnostic ModelPack weight
// media types (e.g., .raw). Falls back to inspecting layer filepath
// annotations when the config omits the format field. Returns empty string if
// format cannot be determined.
func getModelFormat(i WithLayers) string {
	cfg, err := Config(i)
	if err == nil {
		if f := string(cfg.GetFormat()); f != "" {
			return f
		}
	}
	// Config did not specify a format; infer from filepath annotations.
	return inferFormatFromAnnotations(i)
}

// inferFormatFromAnnotations scans weight layers for a filepath annotation and
// uses the file extension to determine the model format. This handles CNCF
// ModelPack models that use MediaTypeWeightRaw but omit config.format.
func inferFormatFromAnnotations(i WithLayers) string {
	layers, err := i.Layers()
	if err != nil {
		return ""
	}
	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil || !modelpack.IsModelPackWeightMediaType(string(mt)) {
			continue
		}
		dp, ok := l.(descriptorProvider)
		if !ok {
			continue
		}
		fp, exists := dp.GetDescriptor().Annotations[types.AnnotationFilePath]
		if !exists || fp == "" {
			continue
		}
		// Use file classification to detect format from extension.
		switch files.Classify(fp) {
		case files.FileTypeGGUF:
			return string(types.FormatGGUF)
		case files.FileTypeSafetensors:
			return string(types.FormatSafetensors)
		case files.FileTypeDDUF:
			return string(types.FormatDDUF)
		case files.FileTypeUnknown, files.FileTypeConfig, files.FileTypeLicense, files.FileTypeChatTemplate:
			// Not a weight file; skip.
		}
	}
	return ""
}

// layerPathsByMediaType is a generic helper function that finds a layer by media type and returns its path.
// Natively supports both Docker and ModelPack media types without any conversion.
// The modelFormat parameter is used to resolve format-agnostic ModelPack weight types (e.g., .raw, .tar)
// to the correct model format. Pass empty string when not needed.
func layerPathsByMediaType(i WithLayers, mediaType oci.MediaType, modelFormat string) ([]string, error) {
	layers, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}
	var paths []string
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			continue
		}
		if !matchesMediaType(mt, mediaType, modelFormat) {
			continue
		}
		layer, ok := l.(*Layer)
		if !ok {
			return nil, fmt.Errorf("%s Layer is not available locally", mediaType)
		}
		paths = append(paths, layer.Path)
	}
	return paths, nil
}

// matchesMediaType checks if a layer media type matches the target type.
// Natively supports both Docker and ModelPack formats without any conversion.
// The modelFormat parameter is used to resolve format-agnostic ModelPack weight types
// (e.g., .raw, .tar) when the format is specified in the model config rather than
// the layer media type. Pass empty string when not needed.
func matchesMediaType(layerMT, targetMT oci.MediaType, modelFormat string) bool {
	// Exact match
	if layerMT == targetMT {
		return true
	}

	// Native ModelPack support: check format-specific and category types.
	switch targetMT {
	case types.MediaTypeGGUF:
		if layerMT == modelpack.MediaTypeWeightGGUF {
			return true
		}
	case types.MediaTypeSafetensors:
		if layerMT == modelpack.MediaTypeWeightSafetensors {
			return true
		}
	case types.MediaTypeLicense:
		// CNCF doc layers may carry license files.
		if layerMT == modelpack.MediaTypeDocRaw {
			return true
		}
	case types.MediaTypeChatTemplate, types.MediaTypeModelFile, types.MediaTypeVLLMConfigArchive:
		// CNCF weight config layers carry config/tokenizer/template files.
		if layerMT == modelpack.MediaTypeWeightConfigRaw {
			return true
		}
	case types.MediaTypeMultimodalProjector:
		// mmproj files are packaged as generic weights in CNCF format.
		// Cannot distinguish from other weight files by media type alone;
		// rely on filepath annotation during unpack.
	case types.MediaTypeDDUF, types.MediaTypeDirTar, types.MediaTypeModelConfigV01,
		types.MediaTypeModelConfigV02,
		oci.OCIManifestSchema1, oci.OCIImageIndex, oci.OCIConfigJSON,
		oci.OCILayer, oci.OCILayerGzip, oci.OCILayerZstd,
		oci.OCIContentDescriptor, oci.OCIArtifactManifest, oci.OCIEmptyJSON,
		oci.DockerManifestSchema2, oci.DockerManifestList, oci.DockerConfigJSON,
		oci.DockerLayer, oci.DockerForeignLayer, oci.DockerUncompressedLayer:
		// No format-specific ModelPack mapping for these media types.
	}

	// ModelPack model-spec support: format-agnostic weight types (.raw, .tar, etc.)
	// Only truly generic/format-agnostic types qualify here. Format-specific types
	// (e.g., MediaTypeWeightGGUF, MediaTypeWeightSafetensors) already encode the format
	// in their media type and are handled above; applying this fallback to them would
	// cause cross-format false positives (e.g., safetensors layer matching as GGUF).
	// MediaTypeWeightConfigRaw and MediaTypeDocRaw are also excluded because they carry
	// non-weight content.
	if modelFormat != "" && modelpack.IsModelPackGenericWeightMediaType(string(layerMT)) {
		switch targetMT {
		case types.MediaTypeGGUF:
			return modelFormat == string(types.FormatGGUF)
		case types.MediaTypeSafetensors:
			return modelFormat == string(types.FormatSafetensors)
		case types.MediaTypeDDUF:
			return modelFormat == string(types.FormatDDUF) || modelFormat == string(types.FormatDiffusers) //nolint:staticcheck // FormatDiffusers kept for backward compatibility
		case types.MediaTypeLicense, types.MediaTypeMultimodalProjector,
			types.MediaTypeChatTemplate, types.MediaTypeModelFile, types.MediaTypeVLLMConfigArchive,
			types.MediaTypeDirTar, types.MediaTypeModelConfigV01, types.MediaTypeModelConfigV02,
			oci.OCIManifestSchema1, oci.OCIImageIndex, oci.OCIConfigJSON,
			oci.OCILayer, oci.OCILayerGzip, oci.OCILayerZstd,
			oci.OCIContentDescriptor, oci.OCIArtifactManifest, oci.OCIEmptyJSON,
			oci.DockerManifestSchema2, oci.DockerManifestList, oci.DockerConfigJSON,
			oci.DockerLayer, oci.DockerForeignLayer, oci.DockerUncompressedLayer:
			// No generic weight resolution for these media types.
		}
	}

	return false
}

// ManifestOptions holds the manifest-level metadata for an artifact.
type ManifestOptions struct {
	// ConfigMediaType is the media type of the config blob.
	ConfigMediaType oci.MediaType
	// ArtifactType is the OCI artifact type of the manifest (optional).
	// The CNCF ModelPack spec requires
	// "application/vnd.cncf.model.manifest.v1+json".
	ArtifactType string
}

// WithManifestOptions provides manifest assembly options.
type WithManifestOptions interface {
	GetManifestOptions() ManifestOptions
}

// resolveManifestOptions extracts manifest options from the given object
// via the WithManifestOptions interface.
func resolveManifestOptions(i interface{}) ManifestOptions {
	if mof, ok := i.(WithManifestOptions); ok {
		return mof.GetManifestOptions()
	}
	return ManifestOptions{}
}

// ManifestForLayers assembles an OCI manifest for the given model. The
// config media type and optional artifact type are read from the model via
// the WithManifestOptions interface.
func ManifestForLayers(i WithLayers) (*oci.Manifest, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}
	cfgHash, _, err := oci.SHA256(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("compute config hash: %w", err)
	}

	// Resolve config media type and artifact type from the model.
	opts := resolveManifestOptions(i)
	configMediaType := opts.ConfigMediaType
	if configMediaType == "" {
		// Default to Docker format V01 for backward compatibility.
		configMediaType = types.MediaTypeModelConfigV01
	}

	cfgDsc := oci.Descriptor{
		MediaType: configMediaType,
		Size:      int64(len(raw)),
		Digest:    cfgHash,
	}

	ls, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}

	var layers []oci.Descriptor
	for _, l := range ls {
		// Check if this is our Layer type which embeds the full descriptor
		// with annotations.
		if layer, ok := l.(*Layer); ok {
			// Use the embedded descriptor directly to preserve annotations.
			layers = append(layers, layer.Descriptor)
		} else if dp, ok := l.(descriptorProvider); ok {
			// Use GetDescriptor() to preserve annotations from wrapper
			// types like remappedLayer.
			layers = append(layers, dp.GetDescriptor())
		} else {
			// Fall back to computing descriptor for other layer types.
			mt, err := l.MediaType()
			if err != nil {
				return nil, fmt.Errorf("get layer media type: %w", err)
			}
			size, err := l.Size()
			if err != nil {
				return nil, fmt.Errorf("get layer size: %w", err)
			}
			digest, err := l.Digest()
			if err != nil {
				return nil, fmt.Errorf("get layer digest: %w", err)
			}
			layers = append(layers, oci.Descriptor{
				MediaType: mt,
				Size:      size,
				Digest:    digest,
			})
		}
	}

	return &oci.Manifest{
		SchemaVersion: 2,
		MediaType:     oci.OCIManifestSchema1,
		ArtifactType:  opts.ArtifactType,
		Config:        cfgDsc,
		Layers:        layers,
	}, nil
}
