package gguf

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/go-units"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	parser "github.com/gpustack/gguf-parser-go"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func NewModel(path string) (*Model, error) {
	shards := parser.CompleteShardGGUFFilename(path)
	if len(shards) == 0 {
		shards = []string{path} // single file
	}
	layers := make([]v1.Layer, len(shards))
	diffIDs := make([]v1.Hash, len(shards))
	for i, shard := range shards {
		layer, err := partial.NewLayer(shard, types.MediaTypeGGUF)
		if err != nil {
			return nil, fmt.Errorf("create gguf layer: %w", err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get gguf layer diffID: %w", err)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	created := time.Now()
	return &Model{
		configFile: types.ConfigFile{
			Config: configFromFile(path),
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: v1.RootFS{
				Type:    "rootfs",
				DiffIDs: diffIDs,
			},
		},
		layers: layers,
	}, nil
}

func configFromFile(path string) types.Config {
	gguf, err := parser.ParseGGUFFile(path)
	if err != nil {
		return types.Config{} // continue without metadata
	}
	
	meta := gguf.Metadata()
	return types.Config{
		Format:       types.FormatGGUF,
		Parameters:   formatParameters(int64(meta.Parameters)),
		Architecture: strings.TrimSpace(meta.Architecture),
		Quantization: strings.TrimSpace(meta.FileType.String()),
		Size:         formatSize(int64(meta.Size)),
		GGUF:         extractGGUFMetadata(&gguf.Header),
	}
}

// formatParameters converts parameter count to human-readable format
// Returns format like "361.82M" or "1.5B" (no space, base 1000, where B = Billion)
func formatParameters(params int64) string {
	return units.CustomSize("%.2f%s", float64(params), 1000.0, []string{"", "K", "M", "B", "T"})
}

// formatSize converts bytes to human-readable format using binary units (base 1024)
// Returns format like "244.45MiB" (no space, matching Docker's format)
func formatSize(bytes int64) string {
	return units.BytesSize(float64(bytes))
}
