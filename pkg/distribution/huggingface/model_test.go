package huggingface

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/distribution/types"
)

// TestBuildGGUFModelV01WithMMProj verifies that buildGGUFModelV01 includes
// the multimodal projector as a MediaTypeMultimodalProjector layer when an
// mmprojFile is provided.
func TestBuildGGUFModelV01WithMMProj(t *testing.T) {
	assetsDir := filepath.Join("..", "assets")
	ggufPath := filepath.Join(assetsDir, "dummy.gguf")
	mmprojPath := filepath.Join(assetsDir, "dummy.mmproj")

	weightFiles := []RepoFile{
		{Type: "file", Path: "dummy.gguf"},
	}
	mmprojFile := &RepoFile{Type: "file", Path: "mmproj-model-f16.gguf"}
	localPaths := map[string]string{
		"dummy.gguf":            ggufPath,
		"mmproj-model-f16.gguf": mmprojPath,
	}

	artifact, err := buildGGUFModelV01(localPaths, weightFiles, nil, mmprojFile, nil)
	if err != nil {
		t.Fatalf("buildGGUFModelV01 failed: %v", err)
	}

	// Retrieve the manifest and look for the mmproj layer.
	manifest, err := artifact.Manifest()
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}

	found := false
	for _, layer := range manifest.Layers {
		if layer.MediaType == types.MediaTypeMultimodalProjector {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected manifest to contain a %s layer, but none was found",
			types.MediaTypeMultimodalProjector)
	}
}

// TestBuildGGUFModelV01WithoutMMProj verifies that buildGGUFModelV01 succeeds
// and produces no MediaTypeMultimodalProjector layer when no mmprojFile is
// provided.
func TestBuildGGUFModelV01WithoutMMProj(t *testing.T) {
	assetsDir := filepath.Join("..", "assets")
	ggufPath := filepath.Join(assetsDir, "dummy.gguf")

	weightFiles := []RepoFile{
		{Type: "file", Path: "dummy.gguf"},
	}
	localPaths := map[string]string{
		"dummy.gguf": ggufPath,
	}
	createdTime := time.Now()

	artifact, err := buildGGUFModelV01(localPaths, weightFiles, nil, nil, &createdTime)
	if err != nil {
		t.Fatalf("buildGGUFModelV01 failed: %v", err)
	}

	manifest, err := artifact.Manifest()
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType == types.MediaTypeMultimodalProjector {
			t.Errorf("expected no %s layer, but one was found",
				types.MediaTypeMultimodalProjector)
		}
	}
}
