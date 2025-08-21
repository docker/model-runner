package gguf

import (
	"fmt"
	"strings"

	parser "github.com/gpustack/gguf-parser-go"
)

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
