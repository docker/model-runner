package mutate_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestAppendLayer(t *testing.T) {
	mdl1 := testutil.NewGGUFArtifact(t, filepath.Join("..", "..", "assets", "dummy.gguf"))
	manifest1, err := mdl1.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if len(manifest1.Layers) != 1 { // begin with one layer
		t.Fatalf("Expected 1 layer, got %d", len(manifest1.Layers))
	}

	// Append a layer
	mdl2 := mutate.AppendLayers(mdl1,
		testutil.NewStaticLayer([]byte("some layer content"), "application/vnd.example.some.media.type"),
	)
	if mdl2 == nil {
		t.Fatal("Expected non-nil model")
	}

	// Check the manifest
	manifest2, err := mdl2.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if len(manifest2.Layers) != 2 { // begin with one layer
		t.Fatalf("Expected 2 layers, got %d", len(manifest1.Layers))
	}

	// Check the config file
	rawCfg, err := mdl2.RawConfigFile()
	if err != nil {
		t.Fatalf("Failed to get raw config file: %v", err)
	}
	var cfg types.ConfigFile
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config file: %v", err)
	}
	if len(cfg.RootFS.DiffIDs) != 2 {
		t.Fatalf("Expected 2 diff ids in rootfs, got %d", len(cfg.RootFS.DiffIDs))
	}
}

func TestConfigMediaTypes(t *testing.T) {
	mdl1 := testutil.NewGGUFArtifact(t, filepath.Join("..", "..", "assets", "dummy.gguf"))
	manifest1, err := mdl1.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if manifest1.Config.MediaType != types.MediaTypeModelConfigV01 {
		t.Fatalf("Expected media type %s, got %s", types.MediaTypeModelConfigV01, manifest1.Config.MediaType)
	}

	newMediaType := oci.MediaType("application/vnd.example.other.type")
	mdl2 := mutate.ConfigMediaType(mdl1, newMediaType)
	manifest2, err := mdl2.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if manifest2.Config.MediaType != newMediaType {
		t.Fatalf("Expected media type %s, got %s", newMediaType, manifest2.Config.MediaType)
	}
}

func TestContextSize(t *testing.T) {
	mdl1 := testutil.NewGGUFArtifact(t, filepath.Join("..", "..", "assets", "dummy.gguf"))
	cfg, err := mdl1.Config()
	if err != nil {
		t.Fatalf("Failed to get config file: %v", err)
	}
	if cfg.GetContextSize() != nil {
		t.Fatalf("Epected nil context size got %d", *cfg.GetContextSize())
	}

	// set the context size
	mdl2 := mutate.ContextSize(mdl1, 2096)

	// check the config
	cfg2, err := mdl2.Config()
	if err != nil {
		t.Fatalf("Failed to get config file: %v", err)
	}
	if cfg2.GetContextSize() == nil {
		t.Fatal("Expected non-nil context")
	}
	if *cfg2.GetContextSize() != 2096 {
		t.Fatalf("Expected context size of 2096 got %d", *cfg2.GetContextSize())
	}
}
