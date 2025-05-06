package gguf

import (
	"fmt"
	"strconv"
	"strings"

	parser "github.com/gpustack/gguf-parser-go"
)

const maxArraySize = 50

// FieldNotFoundError represents an error when a required field is not found in the GGUF file
type FieldNotFoundError struct {
	Field string
}

// Error implements the error interface
func (e *FieldNotFoundError) Error() string {
	return fmt.Sprintf("field not found: %s", e.Field)
}

// NewFieldNotFoundError creates a new FieldNotFoundError
func NewFieldNotFoundError(field string) *FieldNotFoundError {
	return &FieldNotFoundError{Field: field}
}

// File implements the GGUFFile interface
type File struct {
	file *parser.GGUFFile
}

// GetParameters returns the model parameters (raw count, formatted string, error)
func (g *File) GetParameters() (float64, string, error) {
	if g.file == nil {
		return 0, "", fmt.Errorf("file is nil")
	}

	// size_label is the human-readable size of the model
	sizeLabel, found := g.file.Header.MetadataKV.Get("general.size_label")
	if found {
		formattedValue := sizeLabel.ValueString()
		// Parse the formatted value to get the raw value
		rawValue := parseParameters(formattedValue)
		if rawValue != 0 { // Skip non-numeric size labels (e.g. "large" in mxbai-embed-large-v1)
			return rawValue, formattedValue, nil
		}
	}

	// If no size label is found, use the parameters which is the exact number of parameters in the model
	paramsStr := g.file.Metadata().Parameters.String()
	if paramsStr == "" {
		return 0, "", NewFieldNotFoundError("parameters")
	}

	formattedValue := strings.TrimSpace(g.file.Metadata().Parameters.String())
	rawValue := parseParameters(formattedValue)
	return rawValue, formattedValue, nil
}

// GetArchitecture returns the model architecture
func (g *File) GetArchitecture() string {
	return g.file.Metadata().Architecture
}

// GetQuantization returns the model quantization (raw string, formatted string, error)
func (g *File) GetQuantization() parser.GGUFFileType {
	return g.file.Metadata().FileType
}

// GetSize returns the model size (bytes, error)
func (g *File) GetSize() (uint64, error) {
	size := g.file.Metadata().Size
	if size == 0 {
		return 0, NewFieldNotFoundError("size")
	}

	return uint64(size), nil
}

// GetContextLength returns the model context length (raw length, formatted string, error)
func (g *File) GetContextLength() (uint32, error) {
	architecture, found := g.file.Header.MetadataKV.Get("general.architecture")
	if !found {
		return 0, NewFieldNotFoundError("general.architecture")
	}

	contextLength, found := g.file.Header.MetadataKV.Get(architecture.ValueString() + ".context_length")
	if !found {
		return 0, NewFieldNotFoundError(architecture.ValueString() + ".context_length")
	}

	return contextLength.ValueUint32(), nil
}

// GetVRAM returns the estimated VRAM requirements (bytes, error)
func (g *File) GetVRAM() (float64, error) {
	// Get parameter count
	params, _, err := g.GetParameters()
	if err != nil {
		return 0, fmt.Errorf("failed to get parameters: %w", err)
	}

	// Determine quantization
	quantization := g.GetQuantization()
	ggmlType := quantization.GGMLType()
	trait, ok := ggmlType.Trait()
	if !ok {
		return 0, fmt.Errorf("unknown quantization type: %v", quantization)
	}

	// Calculate bytes per parameter based on quantization type
	var bytesPerParam float64
	if trait.Quantized {
		// For quantized types, calculate bytes per parameter based on type size and block size
		bytesPerParam = float64(trait.TypeSize) / float64(trait.BlockSize)
	} else {
		// For non-quantized types, use the type size directly
		bytesPerParam = float64(trait.TypeSize)
	}

	// Extract KV cache dimensions
	arch := g.GetArchitecture()
	nLayer, found := g.file.Header.MetadataKV.Get(arch + ".block_count")
	if !found {
		return 0, NewFieldNotFoundError(arch + ".block_count")
	}
	nEmb, found := g.file.Header.MetadataKV.Get(arch + ".embedding_length")
	if !found {
		return 0, NewFieldNotFoundError(arch + ".embedding_length")
	}

	// Get context length
	contextLength, err := g.GetContextLength()
	if err != nil {
		return 0, fmt.Errorf("failed to get context length: %w", err)
	}

	// Calculate model weights size
	modelSize := params * bytesPerParam
	// Calculate KV cache size
	kvCacheBytes := contextLength * nLayer.ValueUint32() * nEmb.ValueUint32() * 2 * 2
	kvCache := float64(kvCacheBytes)

	// Total VRAM estimate with 20% overhead
	totalVRAM := (modelSize + kvCache) * 1.2
	return totalVRAM, nil
}

// parseParameters converts parameter string to float64
func parseParameters(paramStr string) float64 {
	// Remove any non-numeric characters except decimal point
	toParse := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, paramStr)

	// Parse the number
	params, err := strconv.ParseFloat(toParse, 64)
	if err != nil {
		return 0
	}

	// Convert to actual number of parameters (e.g., 1.24B -> 1.24e9)
	if strings.Contains(strings.ToUpper(paramStr), "B") {
		params *= 1e9
	} else if strings.Contains(strings.ToUpper(paramStr), "M") {
		params *= 1e6
	}

	return params
}

func (g *File) GetMetadata() map[string]string {
	metadata := make(map[string]string)
	for _, kv := range g.file.Header.MetadataKV {
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
			value = handleArray(kv.ValueArray())
		default:
			value = fmt.Sprintf("[unknown type %d]", kv.ValueType)
		}
		metadata[kv.Key] = value
	}
	return metadata
}

// handleArray processes an array value and returns its string representation
func handleArray(arrayValue parser.GGUFMetadataKVArrayValue) string {
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
		default:
			// Do nothing
		}
	}
	return strings.Join(values, ", ")
}
