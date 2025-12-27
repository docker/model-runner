package partial_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// mockConfig is used to test ConfigFile and Config functions
type mockConfig struct {
	raw []byte
	err error
}

func (m *mockConfig) RawConfigFile() ([]byte, error) {
	return m.raw, m.err
}

// TestConfig_NativeFormatSupport tests that Config() returns native format without conversion
func TestConfig_NativeFormatSupport(t *testing.T) {
	t.Run("Docker format returns *types.Config", func(t *testing.T) {
		// Docker format config
		dockerJSON := `{
			"config": {"format": "gguf", "parameters": "8B"},
			"descriptor": {},
			"rootfs": {"type": "layers", "diff_ids": []}
		}`

		mock := &mockConfig{raw: []byte(dockerJSON)}
		cfg, err := partial.Config(mock)
		if err != nil {
			t.Fatalf("Config() error = %v", err)
		}

		if cfg.GetFormat() != types.FormatGGUF {
			t.Errorf("GetFormat() = %v, want %v", cfg.GetFormat(), types.FormatGGUF)
		}
		if cfg.GetParameters() != "8B" {
			t.Errorf("GetParameters() = %q, want %q", cfg.GetParameters(), "8B")
		}
	})

	t.Run("ModelPack format returns *modelpack.Model without conversion", func(t *testing.T) {
		// ModelPack format config (uses paramSize not parameters)
		modelPackJSON := `{
			"descriptor": {"createdAt": "2025-01-15T10:30:00Z"},
			"config": {"format": "gguf", "paramSize": "8B"},
			"modelfs": {"type": "layers", "diffIds": []}
		}`

		mock := &mockConfig{raw: []byte(modelPackJSON)}
		cfg, err := partial.Config(mock)
		if err != nil {
			t.Fatalf("Config() error = %v", err)
		}

		// Should return native format with interface methods working
		if cfg.GetFormat() != types.FormatGGUF {
			t.Errorf("GetFormat() = %v, want %v", cfg.GetFormat(), types.FormatGGUF)
		}
		// GetParameters() returns ParamSize for ModelPack
		if cfg.GetParameters() != "8B" {
			t.Errorf("GetParameters() = %q, want %q", cfg.GetParameters(), "8B")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		mock := &mockConfig{raw: []byte("not valid json")}
		_, err := partial.Config(mock)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

// TestConfigFile tests ConfigFile() which is for Docker format only
func TestConfigFile(t *testing.T) {
	t.Run("Docker format parses correctly", func(t *testing.T) {
		dockerJSON := `{
			"config": {"format": "gguf", "parameters": "8B"},
			"descriptor": {},
			"rootfs": {"type": "layers", "diff_ids": []}
		}`

		mock := &mockConfig{raw: []byte(dockerJSON)}
		cf, err := partial.ConfigFile(mock)
		if err != nil {
			t.Fatalf("ConfigFile() error = %v", err)
		}

		if cf.Config.Format != types.FormatGGUF {
			t.Errorf("Format = %v, want %v", cf.Config.Format, types.FormatGGUF)
		}
		if cf.Config.Parameters != "8B" {
			t.Errorf("Parameters = %q, want %q", cf.Config.Parameters, "8B")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		mock := &mockConfig{raw: []byte("not valid json")}
		_, err := partial.ConfigFile(mock)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestMMPROJPath(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Add multimodal projector layer
	mmprojLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector)
	if err != nil {
		t.Fatalf("Failed to create multimodal projector layer: %v", err)
	}

	mdlWithMMProj := mutate.AppendLayers(mdl, mmprojLayer)

	// Test MMPROJPath function
	mmprojPath, err := partial.MMPROJPath(mdlWithMMProj)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}

	expectedPath := filepath.Join("..", "..", "assets", "dummy.mmproj")
	if mmprojPath != expectedPath {
		t.Errorf("Expected multimodal projector path %s, got %s", expectedPath, mmprojPath)
	}
}

func TestMMPROJPathNotFound(t *testing.T) {
	// Create a model from a GGUF file without a Multimodal projector
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Test MMPROJPath function should return error
	_, err = partial.MMPROJPath(mdl)
	if err == nil {
		t.Error("Expected error when getting multimodal projector path from model without multimodal projector layer")
	}

	expectedErrorMsg := `model does not contain any layer of type "application/vnd.docker.ai.mmproj"`
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrorMsg, err.Error())
	}
}

func TestGGUFPath(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Test GGUFPath function
	ggufPaths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("Failed to get GGUF path: %v", err)
	}

	if len(ggufPaths) != 1 {
		t.Errorf("Expected single gguf path, got %d", len(ggufPaths))
	}

	expectedPath := filepath.Join("..", "..", "assets", "dummy.gguf")
	if ggufPaths[0] != expectedPath {
		t.Errorf("Expected GGUF path %s, got %s", expectedPath, ggufPaths[0])
	}
}

func TestLayerPathByMediaType(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Add license layer
	licenseLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "license.txt"), types.MediaTypeLicense)
	if err != nil {
		t.Fatalf("Failed to create license layer: %v", err)
	}

	// Add a Multimodal projector layer
	mmprojLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector)
	if err != nil {
		t.Fatalf("Failed to create multimodal projector layer: %v", err)
	}

	mdlWithLayers := mutate.AppendLayers(mdl, licenseLayer, mmprojLayer)

	// Test that we can find each layer type
	ggufPaths, err := partial.GGUFPaths(mdlWithLayers)
	if err != nil {
		t.Fatalf("Failed to get GGUF path: %v", err)
	}

	if len(ggufPaths) != 1 {
		t.Fatalf("Expected single gguf path, got %d", len(ggufPaths))
	}
	if ggufPaths[0] != filepath.Join("..", "..", "assets", "dummy.gguf") {
		t.Errorf("Expected GGUF path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.gguf"), ggufPaths[0])
	}

	mmprojPath, err := partial.MMPROJPath(mdlWithLayers)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}
	if mmprojPath != filepath.Join("..", "..", "assets", "dummy.mmproj") {
		t.Errorf("Expected multimodal projector path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.mmproj"), mmprojPath)
	}

}

// TestGGUFPaths_ModelPackMediaType tests that GGUFPaths can find ModelPack format layers
func TestGGUFPaths_ModelPackMediaType(t *testing.T) {
	// Create a layer with ModelPack GGUF media type
	modelPackGGUFType := oci.MediaType("application/vnd.cncf.model.weight.v1.gguf")

	layer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "dummy.gguf"), modelPackGGUFType)
	if err != nil {
		t.Fatalf("Failed to create ModelPack layer: %v", err)
	}

	// Create a model with mutate and add the layer
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create GGUF model: %v", err)
	}

	mdlWithModelPackLayer := mutate.AppendLayers(mdl, layer)

	// GGUFPaths should be able to find ModelPack format GGUF layers
	paths, err := partial.GGUFPaths(mdlWithModelPackLayer)
	if err != nil {
		t.Fatalf("GGUFPaths() error = %v", err)
	}

	// Should find two: original Docker format + newly added ModelPack format
	if len(paths) != 2 {
		t.Errorf("Expected 2 GGUF paths, got %d", len(paths))
	}
}
