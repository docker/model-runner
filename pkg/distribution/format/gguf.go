package format

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	parser "github.com/gpustack/gguf-parser-go"
)

// GGUFFormat implements the Format interface for GGUF model files.
type GGUFFormat struct{}

// init registers the GGUF format implementation.
func init() {
	Register(&GGUFFormat{})
}

// Name returns the format identifier for GGUF.
func (g *GGUFFormat) Name() types.Format {
	return types.FormatGGUF
}

// MediaType returns the OCI media type for GGUF layers.
func (g *GGUFFormat) MediaType() oci.MediaType {
	return types.MediaTypeGGUF
}

// DiscoverShards finds all GGUF shard files for a sharded model.
// GGUF shards follow the pattern: <name>-00001-of-00015.gguf
// For single-file models, returns a slice containing only the input path.
func (g *GGUFFormat) DiscoverShards(path string) ([]string, error) {
	// Use the external GGUF parser's shard discovery
	shards := parser.CompleteShardGGUFFilename(path)
	if len(shards) == 0 {
		// Single file, not sharded
		return []string{path}, nil
	}
	return shards, nil
}

// ExtractConfig parses GGUF file(s) and extracts model configuration metadata.
func (g *GGUFFormat) ExtractConfig(paths []string) (types.Config, error) {
	if len(paths) == 0 {
		return types.Config{Format: types.FormatGGUF}, nil
	}

	// Parse the first shard/file to get metadata
	gguf, err := parser.ParseGGUFFile(paths[0])
	if err != nil {
		// Return empty config if parsing fails, continue without metadata
		return types.Config{Format: types.FormatGGUF}, nil
	}

	return types.Config{
		Format:       types.FormatGGUF,
		Parameters:   normalizeUnitString(gguf.Metadata().Parameters.String()),
		Architecture: strings.TrimSpace(gguf.Metadata().Architecture),
		Quantization: strings.TrimSpace(gguf.Metadata().FileType.String()),
		Size:         normalizeUnitString(gguf.Metadata().Size.String()),
		GGUF:         extractGGUFMetadata(&gguf.Header),
	}, nil
}

var (
	// spaceBeforeUnitRegex matches one or more spaces between a valid number and a letter (unit)
	// Used to remove spaces between numbers and units (e.g., "16.78 M" -> "16.78M")
	spaceBeforeUnitRegex = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s+([A-Za-z]+)`)
)

// normalizeUnitString removes spaces between numbers and units for consistent formatting
// Examples: "16.78 M" -> "16.78M", "256.35 MiB" -> "256.35MiB", "409M" -> "409M"
func normalizeUnitString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return spaceBeforeUnitRegex.ReplaceAllString(s, "$1$2")
}

const maxArraySize = 50

// extractGGUFMetadata converts the GGUF header metadata into a string map.
func extractGGUFMetadata(header *parser.GGUFHeader) map[string]string {
	metadata := make(map[string]string)

	for _, kv := range header.MetadataKV {
		if kv.ValueType == parser.GGUFMetadataValueTypeArray {
			arrayValue := kv.ValueArray()
			if arrayValue.Len > maxArraySize {
				continue
			}
		}
		var value string
		switch kv.ValueType {
		case parser.GGUFMetadataValueTypeUint8:
			value = fmt.Sprintf("%d", kv.ValueUint8())
		case parser.GGUFMetadataValueTypeInt8:
			value = fmt.Sprintf("%d", kv.ValueInt8())
		case parser.GGUFMetadataValueTypeUint16:
			value = fmt.Sprintf("%d", kv.ValueUint16())
		case parser.GGUFMetadataValueTypeInt16:
			value = fmt.Sprintf("%d", kv.ValueInt16())
		case parser.GGUFMetadataValueTypeUint32:
			value = fmt.Sprintf("%d", kv.ValueUint32())
		case parser.GGUFMetadataValueTypeInt32:
			value = fmt.Sprintf("%d", kv.ValueInt32())
		case parser.GGUFMetadataValueTypeUint64:
			value = fmt.Sprintf("%d", kv.ValueUint64())
		case parser.GGUFMetadataValueTypeInt64:
			value = fmt.Sprintf("%d", kv.ValueInt64())
		case parser.GGUFMetadataValueTypeFloat32:
			value = fmt.Sprintf("%f", kv.ValueFloat32())
		case parser.GGUFMetadataValueTypeFloat64:
			value = fmt.Sprintf("%f", kv.ValueFloat64())
		case parser.GGUFMetadataValueTypeBool:
			value = fmt.Sprintf("%t", kv.ValueBool())
		case parser.GGUFMetadataValueTypeString:
			value = kv.ValueString()
		case parser.GGUFMetadataValueTypeArray:
			value = handleGGUFArray(kv.ValueArray())
		default:
			value = fmt.Sprintf("[unknown type %d]", kv.ValueType)
		}
		metadata[kv.Key] = value
	}

	return metadata
}

// handleGGUFArray processes an array value and returns its string representation.
func handleGGUFArray(arrayValue parser.GGUFMetadataKVArrayValue) string {
	var values []string
	for _, v := range arrayValue.Array {
		switch arrayValue.Type {
		case parser.GGUFMetadataValueTypeUint8:
			values = append(values, fmt.Sprintf("%d", v.(uint8)))
		case parser.GGUFMetadataValueTypeInt8:
			values = append(values, fmt.Sprintf("%d", v.(int8)))
		case parser.GGUFMetadataValueTypeUint16:
			values = append(values, fmt.Sprintf("%d", v.(uint16)))
		case parser.GGUFMetadataValueTypeInt16:
			values = append(values, fmt.Sprintf("%d", v.(int16)))
		case parser.GGUFMetadataValueTypeUint32:
			values = append(values, fmt.Sprintf("%d", v.(uint32)))
		case parser.GGUFMetadataValueTypeInt32:
			values = append(values, fmt.Sprintf("%d", v.(int32)))
		case parser.GGUFMetadataValueTypeUint64:
			values = append(values, fmt.Sprintf("%d", v.(uint64)))
		case parser.GGUFMetadataValueTypeInt64:
			values = append(values, fmt.Sprintf("%d", v.(int64)))
		case parser.GGUFMetadataValueTypeFloat32:
			values = append(values, fmt.Sprintf("%f", v.(float32)))
		case parser.GGUFMetadataValueTypeFloat64:
			values = append(values, fmt.Sprintf("%f", v.(float64)))
		case parser.GGUFMetadataValueTypeBool:
			values = append(values, fmt.Sprintf("%t", v.(bool)))
		case parser.GGUFMetadataValueTypeString:
			values = append(values, v.(string))
		case parser.GGUFMetadataValueTypeArray:
			// Nested arrays not supported
		}
	}

	return strings.Join(values, ", ")
}
