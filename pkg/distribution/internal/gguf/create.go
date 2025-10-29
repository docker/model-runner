package gguf

import (
	"fmt"
	"strings"
	"time"

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
	return types.Config{
		Format:       types.FormatGGUF,
		Parameters:   strings.TrimSpace(gguf.Metadata().Parameters.String()),
		Architecture: strings.TrimSpace(gguf.Metadata().Architecture),
		Quantization: strings.TrimSpace(gguf.Metadata().FileType.String()),
		Size:         strings.TrimSpace(gguf.Metadata().Size.String()),
		GGUF:         extractGGUFMetadata(&gguf.Header),
	}
}
