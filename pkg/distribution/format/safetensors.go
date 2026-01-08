package format

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// SafetensorsFormat implements the Format interface for Safetensors model files.
type SafetensorsFormat struct{}

// init registers the Safetensors format implementation.
func init() {
	Register(&SafetensorsFormat{})
}

// Name returns the format identifier for Safetensors.
func (s *SafetensorsFormat) Name() types.Format {
	return types.FormatSafetensors
}

// MediaType returns the OCI media type for Safetensors layers.
func (s *SafetensorsFormat) MediaType() oci.MediaType {
	return types.MediaTypeSafetensors
}

var (
	// shardPattern matches safetensors shard filenames like "model-00001-of-00003.safetensors"
	// This pattern assumes 5-digit zero-padded numbering (e.g., 00001-of-00003), which is
	// the most common format used by popular model repositories.
	shardPattern = regexp.MustCompile(`^(.+)-(\d{5})-of-(\d{5})\.safetensors$`)
)

// DiscoverShards finds all Safetensors shard files for a sharded model.
// Safetensors shards follow the pattern: <name>-00001-of-00003.safetensors
// For single-file models, returns a slice containing only the input path.
func (s *SafetensorsFormat) DiscoverShards(path string) ([]string, error) {
	baseName := filepath.Base(path)
	matches := shardPattern.FindStringSubmatch(baseName)

	if len(matches) != 4 {
		// Not a sharded file, return single path
		return []string{path}, nil
	}

	prefix := matches[1]
	totalShards, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("parse shard count: %w", err)
	}

	dir := filepath.Dir(path)
	var shards []string

	// Look for all shards in the same directory
	for i := 1; i <= totalShards; i++ {
		shardName := fmt.Sprintf("%s-%05d-of-%05d.safetensors", prefix, i, totalShards)
		shardPath := filepath.Join(dir, shardName)

		// Check if the file exists
		if _, err := os.Stat(shardPath); err == nil {
			shards = append(shards, shardPath)
		}
	}

	// Return error if we didn't find all expected shards
	if len(shards) != totalShards {
		return nil, fmt.Errorf("incomplete shard set: found %d of %d shards for %s", len(shards), totalShards, baseName)
	}

	// Sort to ensure consistent ordering
	sort.Strings(shards)

	return shards, nil
}

// ExtractConfig parses Safetensors file(s) and extracts model configuration metadata.
func (s *SafetensorsFormat) ExtractConfig(paths []string) (types.Config, error) {
	if len(paths) == 0 {
		return types.Config{Format: types.FormatSafetensors}, nil
	}

	// Parse the first safetensors file to extract metadata
	header, err := parseSafetensorsHeader(paths[0])
	if err != nil {
		// Continue without metadata if parsing fails
		return types.Config{Format: types.FormatSafetensors}, nil
	}

	// Calculate total size across all files
	var totalSize int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return types.Config{}, fmt.Errorf("failed to stat file %s: %w", path, err)
		}
		totalSize += info.Size()
	}

	// Calculate parameters
	params := header.calculateParameters()

	// Extract architecture from metadata if available
	architecture := ""
	if arch, ok := header.Metadata["architecture"]; ok {
		architecture = fmt.Sprintf("%v", arch)
	}

	return types.Config{
		Format:       types.FormatSafetensors,
		Parameters:   formatParameters(params),
		Quantization: header.getQuantization(),
		Size:         formatSize(totalSize),
		Architecture: architecture,
		Safetensors:  header.extractMetadata(),
	}, nil
}

const (
	quantizationUnknown = "unknown"
	quantizationMixed   = "mixed"
)

// safetensorsHeader represents the JSON header in a safetensors file
type safetensorsHeader struct {
	Metadata map[string]interface{}
	Tensors  map[string]tensorInfo
}

// tensorInfo contains information about a tensor
type tensorInfo struct {
	Dtype       string
	Shape       []int64
	DataOffsets [2]int64
}

// parseSafetensorsHeader reads only the header from a safetensors file without loading the entire file.
func parseSafetensorsHeader(path string) (*safetensorsHeader, error) {
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
	tensors := make(map[string]tensorInfo)
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
			for index, v := range shapeArray {
				floatVal, ok := v.(float64)
				if !ok {
					return nil, fmt.Errorf("invalid shape value for tensor %q at index %d: expected number, got %T", name, index, v)
				}
				shape = append(shape, int64(floatVal))
			}
		}

		// Parse data_offsets
		var dataOffsets [2]int64
		if offsetsArray, ok := tensorMap["data_offsets"].([]interface{}); ok {
			if len(offsetsArray) != 2 {
				return nil, fmt.Errorf("invalid data_offsets for tensor %q: expected 2 elements, got %d", name, len(offsetsArray))
			}
			for index, offset := range offsetsArray {
				floatVal, ok := offset.(float64)
				if !ok {
					return nil, fmt.Errorf("invalid data_offsets value for tensor %q at index %d: expected number, got %T", name, index, offset)
				}
				dataOffsets[index] = int64(floatVal)
			}
		}

		tensors[name] = tensorInfo{
			Dtype:       dtype,
			Shape:       shape,
			DataOffsets: dataOffsets,
		}
	}

	return &safetensorsHeader{
		Metadata: metadata,
		Tensors:  tensors,
	}, nil
}

// calculateParameters sums up all tensor parameters
func (h *safetensorsHeader) calculateParameters() int64 {
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

// getQuantization determines the quantization type from tensor dtypes
func (h *safetensorsHeader) getQuantization() string {
	if len(h.Tensors) == 0 {
		return quantizationUnknown
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
		return quantizationUnknown
	}

	// If all tensors have the same dtype, return it
	if len(dtypeCounts) == 1 {
		for dtype := range dtypeCounts {
			return dtype
		}
	}

	return quantizationMixed
}

// extractMetadata converts header to string map (similar to GGUF)
func (h *safetensorsHeader) extractMetadata() map[string]string {
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
// Returns format like "361.82M" or "1.5B" (no space before unit, base 1000, where B = Billion)
func formatParameters(params int64) string {
	return units.CustomSize("%.2f%s", float64(params), 1000.0, []string{"", "K", "M", "B", "T"})
}

// formatSize converts bytes to human-readable format matching Docker's style
// Returns format like "256MB" (decimal units, no space, matching `docker images`)
func formatSize(bytes int64) string {
	return units.CustomSize("%.2f%s", float64(bytes), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"})
}
