package modelpack

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/opencontainers/go-digest"
)

// IsModelPackMediaType checks if the given media type indicates a CNCF ModelPack format.
// It returns true if the media type has the CNCF model prefix.
func IsModelPackMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, MediaTypePrefix)
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

// MapLayerMediaType maps ModelPack layer media types to Docker format.
// Returns the original value if not a ModelPack type.
func MapLayerMediaType(mediaType string) string {
	// Only process ModelPack weight layers
	if !strings.HasPrefix(mediaType, MediaTypePrefix) {
		return mediaType
	}

	// Determine corresponding Docker type based on media type format
	switch {
	case strings.Contains(mediaType, "weight") && strings.Contains(mediaType, "gguf"):
		return string(types.MediaTypeGGUF)
	case strings.Contains(mediaType, "weight") && strings.Contains(mediaType, "safetensors"):
		return string(types.MediaTypeSafetensors)
	default:
		// Keep other layer types (doc, code, etc.) as-is
		return mediaType
	}
}

// ConvertToDockerConfig converts a raw ModelPack config JSON to Docker model-spec ConfigFile.
// It maps common fields directly. Note: Extended ModelPack metadata is not preserved
// since types.Config no longer has a ModelPack field.
func ConvertToDockerConfig(rawConfig []byte) (*types.ConfigFile, error) {
	var mp Model
	if err := json.Unmarshal(rawConfig, &mp); err != nil {
		return nil, fmt.Errorf("unmarshal modelpack config: %w", err)
	}

	// Build the Docker format config
	dockerConfig := &types.ConfigFile{
		Config: types.Config{
			Format:       convertFormat(mp.Config.Format),
			Architecture: mp.Config.Architecture,
			Quantization: mp.Config.Quantization,
			Parameters:   mp.Config.ParamSize,
			Size:         "0", // ModelPack doesn't have an equivalent field
		},
		Descriptor: types.Descriptor{
			Created: mp.Descriptor.CreatedAt,
		},
		RootFS: oci.RootFS{
			Type:    normalizeRootFSType(mp.ModelFS.Type),
			DiffIDs: convertDiffIDs(mp.ModelFS.DiffIDs),
		},
	}

	return dockerConfig, nil
}

// convertFormat maps ModelPack format strings to Docker Format type.
// Format strings are normalized to lowercase for consistent matching.
func convertFormat(mpFormat string) types.Format {
	switch strings.ToLower(mpFormat) {
	case "gguf":
		return types.FormatGGUF
	case "safetensors":
		return types.FormatSafetensors
	default:
		// Pass through unknown formats as-is
		return types.Format(strings.ToLower(mpFormat))
	}
}

// normalizeRootFSType ensures the rootfs type is set correctly.
// ModelPack uses "layers" as the type, which maps to Docker's "layers".
func normalizeRootFSType(mpType string) string {
	if mpType == "" {
		return "layers"
	}
	return mpType
}

// convertDiffIDs converts opencontainers digest.Digest slice to oci.Hash slice.
// Note: Invalid digests are silently skipped here because they will be caught
// during layer validation when the model is actually loaded. This avoids
// failing early for formats we might not fully understand yet.
func convertDiffIDs(digests []digest.Digest) []oci.Hash {
	if len(digests) == 0 {
		return nil
	}

	result := make([]oci.Hash, 0, len(digests))
	for _, d := range digests {
		// digest.Digest format is "algorithm:hex", same as oci.Hash
		hash, err := oci.NewHash(d.String())
		if err != nil {
			// Skip invalid digests; they will be caught during layer validation
			continue
		}
		result = append(result, hash)
	}
	return result
}
