package gguf

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	parser "github.com/gpustack/gguf-parser-go"
)

func NewModel(path string) (*Model, error) {
	shards := parser.CompleteShardGGUFFilename(path)
	if len(shards) == 0 {
		shards = []string{path} // single file
	}
	layers := make([]oci.Layer, len(shards))
	diffIDs := make([]oci.Hash, len(shards))
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
		BaseModel: partial.BaseModel{
			ModelConfigFile: types.ConfigFile{
				Config: configFromFile(path),
				Descriptor: types.Descriptor{
					Created: &created,
				},
				RootFS: oci.RootFS{
					Type:    "rootfs",
					DiffIDs: diffIDs,
				},
			},
			LayerList: layers,
		},
	}, nil
}

func configFromFile(path string) types.Config {
	gguf, err := parser.ParseGGUFFile(path)
	if err != nil {
		return types.Config{} // continue without metadata
	}
	return types.Config{
		Format:       types.FormatGGUF,
		Parameters:   normalizeUnitString(gguf.Metadata().Parameters.String()),
		Architecture: strings.TrimSpace(gguf.Metadata().Architecture),
		Quantization: strings.TrimSpace(gguf.Metadata().FileType.String()),
		Size:         normalizeUnitString(gguf.Metadata().Size.String()),
		GGUF:         extractGGUFMetadata(&gguf.Header),
	}
}

var (
	// spaceBeforeUnitRegex matches one or more spaces between a valid number and a letter (unit)
	// Used to remove spaces between numbers and units (e.g., "16.78 M" -> "16.78M")
	// Pattern: integer or decimal number, then whitespace, then letters (unit)
	spaceBeforeUnitRegex = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s+([A-Za-z]+)`)
)

// normalizeUnitString removes spaces between numbers and units for consistent formatting
// Examples: "16.78 M" -> "16.78M", "256.35 MiB" -> "256.35MiB", "409M" -> "409M"
func normalizeUnitString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Remove space(s) between numbers/decimals and unit letters using regex
	// Pattern matches: number(s) or decimal, then whitespace, then letters (unit)
	return spaceBeforeUnitRegex.ReplaceAllString(s, "$1$2")
}
