package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/docker/go-units"
)

const (
	QuantizationUnknown = "unknown"
	QuantizationMixed   = "mixed"
)

// Header represents the JSON header in a safetensors file
type Header struct {
	Metadata map[string]interface{}
	Tensors  map[string]TensorInfo
}

// TensorInfo contains information about a tensor
type TensorInfo struct {
	Dtype       string
	Shape       []int64
	DataOffsets [2]int64
}

// ParseSafetensorsHeader reads only the header from a safetensors file without loading the entire file.
// This is memory-efficient for large model files (which can be many GB).
//
// Safetensors format:
//
//	[8 bytes: header length (uint64, little-endian)]
//	[N bytes: JSON header]
//	[remaining: tensor data]
func ParseSafetensorsHeader(path string) (*Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Read the first 8 bytes to get the header length
	var headerLen uint64
	if err := binary.Read(file, binary.LittleEndian, &headerLen); err != nil {
		return nil, fmt.Errorf("read header length: %w", err)
	}

	// Sanity check: header shouldn't be larger than 100MB
	if headerLen > 100*1024*1024 {
		return nil, fmt.Errorf("header length too large: %d bytes", headerLen)
	}

	// Read only the header JSON (not the entire file!)
	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(file, headerBytes); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Parse the JSON header
	var rawHeader map[string]interface{}
	if err := json.Unmarshal(headerBytes, &rawHeader); err != nil {
		return nil, fmt.Errorf("parse JSON header: %w", err)
	}

	// Extract metadata (stored under "__metadata__" key)
	var metadata map[string]interface{}
	if rawMetadata, ok := rawHeader["__metadata__"].(map[string]interface{}); ok {
		metadata = rawMetadata
		delete(rawHeader, "__metadata__")
	}

	// Parse tensor info from remaining keys
	tensors := make(map[string]TensorInfo)
	for name, value := range rawHeader {
		tensorMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Parse dtype
		dtype, _ := tensorMap["dtype"].(string)

		// Parse shape
		var shape []int64
		if shapeArray, ok := tensorMap["shape"].([]interface{}); ok {
			for _, v := range shapeArray {
				if floatVal, ok := v.(float64); ok {
					shape = append(shape, int64(floatVal))
				}
			}
		}

		// Parse data_offsets
		var dataOffsets [2]int64
		if offsetsArray, ok := tensorMap["data_offsets"].([]interface{}); ok && len(offsetsArray) == 2 {
			if start, ok := offsetsArray[0].(float64); ok {
				dataOffsets[0] = int64(start)
			}
			if end, ok := offsetsArray[1].(float64); ok {
				dataOffsets[1] = int64(end)
			}
		}

		tensors[name] = TensorInfo{
			Dtype:       dtype,
			Shape:       shape,
			DataOffsets: dataOffsets,
		}
	}

	return &Header{
		Metadata: metadata,
		Tensors:  tensors,
	}, nil
}

// CalculateParameters sums up all tensor parameters
func (h *Header) CalculateParameters() int64 {
	var total int64
	for _, tensor := range h.Tensors {
		params := int64(1)
		for _, dim := range tensor.Shape {
			params *= dim
		}
		total += params
	}
	return total
}

// GetQuantization determines the quantization type from tensor dtypes
func (h *Header) GetQuantization() string {
	if len(h.Tensors) == 0 {
		return QuantizationUnknown
	}

	// Count dtype occurrences (skip empty dtypes)
	dtypeCounts := make(map[string]int)
	for _, tensor := range h.Tensors {
		if tensor.Dtype != "" {
			dtypeCounts[tensor.Dtype]++
		}
	}

	// No valid dtypes found
	if len(dtypeCounts) == 0 {
		return QuantizationUnknown
	}

	// If all tensors have the same dtype, return it
	if len(dtypeCounts) == 1 {
		for dtype := range dtypeCounts {
			return dtype
		}
	}

	return QuantizationMixed
}

// ExtractMetadata converts header to string map (similar to GGUF)
func (h *Header) ExtractMetadata() map[string]string {
	metadata := make(map[string]string)

	// Add metadata from __metadata__ section
	if h.Metadata != nil {
		for k, v := range h.Metadata {
			metadata[k] = fmt.Sprintf("%v", v)
		}
	}

	// Add tensor count
	metadata["tensor_count"] = fmt.Sprintf("%d", len(h.Tensors))

	return metadata
}

// formatParameters converts parameter count to human-readable format matching GGUF style
// Returns format like "361.82 M" or "1.5 B" (space before unit, base 1000, where B = Billion)
func formatParameters(params int64) string {
	return units.CustomSize("%.2f%s", float64(params), 1000.0, []string{"", " K", " M", " B", " T"})
}

// formatSize converts bytes to human-readable format
func formatSize(bytes int64) string {
	return units.HumanSizeWithPrecision(float64(bytes), 2)
}
