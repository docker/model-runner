// Package modelpack provides native support for CNCF ModelPack format models.
// It enables docker/model-runner to pull, store, and run models in ModelPack format
// without conversion. Both Docker and ModelPack formats are supported natively through
// the types.ModelConfig interface.
//
// The struct types (ModelDescriptor, ModelConfig, ModelFS, ModelCapabilities) are
// re-exported directly from the official CNCF model-spec Go module so that
// serialization tags and field definitions stay in sync with the specification.
//
// See: https://github.com/modelpack/model-spec
package modelpack

import (
	"encoding/json"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/types"
	specv1 "github.com/modelpack/model-spec/specs-go/v1"
	"github.com/opencontainers/go-digest"
)

const (
	// MediaTypePrefix is the prefix for all CNCF model config media types.
	MediaTypePrefix = "application/vnd.cncf.model."

	// MediaTypeWeightPrefix is the prefix for all CNCF model weight media types.
	MediaTypeWeightPrefix = "application/vnd.cncf.model.weight."

	// MediaTypeModelConfigV1 is the CNCF model config v1 media type.
	MediaTypeModelConfigV1 = specv1.MediaTypeModelConfig

	// ArtifactTypeModelManifest is the CNCF model manifest artifact type.
	// Required on the manifest when producing model-spec artifacts.
	ArtifactTypeModelManifest = specv1.ArtifactTypeModelManifest

	// MediaTypeWeightRaw is the CNCF model-spec media type for unarchived,
	// uncompressed model weights. This is the type used by modctl and the
	// official model-spec (v0.0.7+).
	MediaTypeWeightRaw = specv1.MediaTypeModelWeightRaw

	// MediaTypeWeightConfigRaw is the CNCF model-spec media type for
	// unarchived, uncompressed weight config files (tokenizer.json,
	// config.json, chat templates, etc.).
	MediaTypeWeightConfigRaw = specv1.MediaTypeModelWeightConfigRaw

	// MediaTypeDocRaw is the CNCF model-spec media type for unarchived,
	// uncompressed documentation files (README.md, LICENSE, etc.).
	MediaTypeDocRaw = specv1.MediaTypeModelDocRaw

	// MediaTypeWeightGGUF is the CNCF ModelPack media type for GGUF weight
	// layers. This is a DMR extension not in the official model-spec; kept
	// for read-compatibility with artifacts produced by older DMR versions.
	MediaTypeWeightGGUF = "application/vnd.cncf.model.weight.v1.gguf"

	// MediaTypeWeightSafetensors is the CNCF ModelPack media type for
	// safetensors weight layers. This is a DMR extension not in the official
	// model-spec; kept for read-compatibility with older DMR artifacts.
	MediaTypeWeightSafetensors = "application/vnd.cncf.model.weight.v1.safetensors"
)

// Type aliases re-export the canonical CNCF model-spec struct types so that
// callers use the upstream definitions (and their JSON tags) by default.
// This eliminates local struct duplication while keeping the modelpack
// package as the single import for DMR code.
type (
	// ModelDescriptor defines the general information of a model.
	ModelDescriptor = specv1.ModelDescriptor

	// ModelConfig defines the execution parameters for an inference engine.
	ModelConfig = specv1.ModelConfig

	// ModelFS describes the layer content addresses.
	ModelFS = specv1.ModelFS

	// ModelCapabilities defines the special capabilities that the model supports.
	ModelCapabilities = specv1.ModelCapabilities
)

// Model represents the CNCF ModelPack config structure.
// It provides the `application/vnd.cncf.model.config.v1+json` mediatype when marshalled to JSON.
//
// The struct mirrors specv1.Model but is declared as its own named type so
// that it can implement the types.ModelConfig interface required by DMR.
type Model struct {
	// Descriptor provides metadata about the model provenance and identity.
	Descriptor ModelDescriptor `json:"descriptor"`

	// ModelFS describes the layer content addresses.
	ModelFS ModelFS `json:"modelfs"`

	// Config defines the execution parameters for the model.
	Config ModelConfig `json:"config,omitempty"`
}

// IsModelPackWeightMediaType checks if the given media type is a CNCF ModelPack weight layer type.
// This includes both format-specific types (e.g., .gguf, .safetensors) and
// format-agnostic types from the official model-spec (e.g., .raw, .tar).
func IsModelPackWeightMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, MediaTypeWeightPrefix)
}

// IsModelPackGenericWeightMediaType checks if the given media type is a format-agnostic
// CNCF ModelPack weight layer type (e.g., MediaTypeWeightRaw).
// Unlike IsModelPackWeightMediaType, this returns false for format-specific types
// like MediaTypeWeightGGUF or MediaTypeWeightSafetensors, which already encode the
// format in the media type itself and must not be matched via the model config format.
// Use this when the actual format must be inferred from the model config rather than
// the layer media type.
func IsModelPackGenericWeightMediaType(mediaType string) bool {
	switch mediaType {
	case MediaTypeWeightRaw:
		return true
	default:
		return false
	}
}

// IsModelPackConfig detects if raw config bytes are in ModelPack format.
// It parses the JSON structure for precise detection, avoiding false positives from string matching.
// ModelPack format characteristics: config.paramSize or descriptor.createdAt
// Docker format uses: config.parameters and descriptor.created
func IsModelPackConfig(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}

	// Parse as map to check actual JSON structure
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return false
	}

	// Check for config.paramSize (ModelPack-specific field)
	if configRaw, ok := parsed["config"]; ok {
		var config map[string]json.RawMessage
		if err := json.Unmarshal(configRaw, &config); err == nil {
			if _, hasParamSize := config["paramSize"]; hasParamSize {
				return true
			}
		}
	}

	// Check for descriptor.createdAt (ModelPack uses camelCase)
	if descRaw, ok := parsed["descriptor"]; ok {
		var desc map[string]json.RawMessage
		if err := json.Unmarshal(descRaw, &desc); err == nil {
			if _, hasCreatedAt := desc["createdAt"]; hasCreatedAt {
				return true
			}
		}
	}

	// Check for modelfs (ModelPack-specific field name)
	if _, hasModelFS := parsed["modelfs"]; hasModelFS {
		return true
	}

	return false
}

// Ensure Model implements types.ModelConfig
var _ types.ModelConfig = (*Model)(nil)

// GetFormat returns the model format, converted to types.Format.
func (m *Model) GetFormat() types.Format {
	f := strings.ToLower(m.Config.Format)
	switch f {
	case "gguf":
		return types.FormatGGUF
	case "safetensors":
		return types.FormatSafetensors
	case "dduf":
		return types.FormatDDUF
	case "diffusers":
		return types.FormatDiffusers //nolint:staticcheck // FormatDiffusers kept for backward compatibility
	default:
		return types.Format(f)
	}
}

// GetContextSize returns the context size. ModelPack spec does not define this field,
// so it always returns nil.
func (m *Model) GetContextSize() *int32 {
	return nil
}

// GetSize returns the parameter size (e.g., "8b").
func (m *Model) GetSize() string {
	return m.Config.ParamSize
}

// GetArchitecture returns the model architecture.
func (m *Model) GetArchitecture() string {
	return m.Config.Architecture
}

// GetParameters returns the parameters description.
// ModelPack uses ParamSize instead of Parameters, so return ParamSize.
func (m *Model) GetParameters() string {
	return m.Config.ParamSize
}

// GetQuantization returns the quantization method.
func (m *Model) GetQuantization() string {
	return m.Config.Quantization
}

// HashToDigest converts a hash string (in "algorithm:hex" form) to a
// digest.Digest. This allows callers to pass oci.Hash.String() values
// without importing the oci package from modelpack.
func HashToDigest(hashStr string) digest.Digest {
	return digest.Digest(hashStr)
}
