package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	// Collect unique dtypes
	dtypes := make(map[string]bool)
	for _, tensor := range h.Tensors {
		dtypes[tensor.Dtype] = true
	}

	// If all tensors have the same dtype, return it
	if len(dtypes) == 1 {
		for dtype := range dtypes {
			return dtype
		}
	}

	// If multiple dtypes, return "mixed"
	// TODO return most common dtype instead
	return "mixed"
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

// formatParameters converts parameter count to human-readable format
func formatParameters(params int64) string {
	if params >= 1_000_000_000 {
		return fmt.Sprintf("%.2f B", float64(params)/1_000_000_000)
	} else if params >= 1_000_000 {
		return fmt.Sprintf("%.2f M", float64(params)/1_000_000)
	} else if params >= 1_000 {
		return fmt.Sprintf("%.2f K", float64(params)/1_000)
	}
	return fmt.Sprintf("%d", params)
}

// formatSize converts bytes to human-readable format
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	if bytes >= GB {
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	} else if bytes >= MB {
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	} else if bytes >= KB {
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%d bytes", bytes)
}
