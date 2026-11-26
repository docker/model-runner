package partial_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
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
	mdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector),
	)

	// Test MMPROJPath function
	mmprojPath, err := partial.MMPROJPath(mdl)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}

	expectedPath := filepath.Join("..", "..", "assets", "dummy.mmproj")
	if mmprojPath != expectedPath {
		t.Errorf("Expected multimodal projector path %s, got %s", expectedPath, mmprojPath)
	}
}

func TestMMPROJPathNotFound(t *testing.T) {
	mdl := testutil.NewGGUFArtifact(t, filepath.Join("..", "..", "assets", "dummy.gguf"))

	// Test MMPROJPath function should return error
	_, err := partial.MMPROJPath(mdl)
	if err == nil {
		t.Error("Expected error when getting multimodal projector path from model without multimodal projector layer")
	}

	expectedErrorMsg := `model does not contain any layer of type "application/vnd.docker.ai.mmproj"`
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrorMsg, err.Error())
	}
}

func TestGGUFPath(t *testing.T) {
	mdl := testutil.NewGGUFArtifact(t, filepath.Join("..", "..", "assets", "dummy.gguf"))

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
	mdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "..", "assets", "license.txt"), types.MediaTypeLicense),
		testutil.Layer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector),
	)

	// Test that we can find each layer type
	ggufPaths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("Failed to get GGUF path: %v", err)
	}

	if len(ggufPaths) != 1 {
		t.Fatalf("Expected single gguf path, got %d", len(ggufPaths))
	}
	if ggufPaths[0] != filepath.Join("..", "..", "assets", "dummy.gguf") {
		t.Errorf("Expected GGUF path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.gguf"), ggufPaths[0])
	}

	mmprojPath, err := partial.MMPROJPath(mdl)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}
	if mmprojPath != filepath.Join("..", "..", "assets", "dummy.mmproj") {
		t.Errorf("Expected multimodal projector path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.mmproj"), mmprojPath)
	}

}

// TestGGUFPaths_ModelPackMediaType tests that GGUFPaths can find ModelPack format layers
func TestGGUFPaths_ModelPackMediaType(t *testing.T) {
	mdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "..", "assets", "dummy.gguf"), oci.MediaType("application/vnd.cncf.model.weight.v1.gguf")),
	)

	// GGUFPaths should be able to find ModelPack format GGUF layers
	paths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("GGUFPaths() error = %v", err)
	}

	// Should find two: original Docker format + newly added ModelPack format
	if len(paths) != 2 {
		t.Errorf("Expected 2 GGUF paths, got %d", len(paths))
	}
}

// TestGGUFPaths_ModelPackRawMediaType tests that GGUFPaths can find layers with
// the real CNCF model-spec format-agnostic media type (application/vnd.cncf.model.weight.v1.raw)
// when the model config specifies format as "gguf".
func TestGGUFPaths_ModelPackRawMediaType(t *testing.T) {
	mdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "..", "assets", "dummy.gguf"), oci.MediaType("application/vnd.cncf.model.weight.v1.raw")),
	)

	// GGUFPaths should find both: original Docker GGUF + raw ModelPack layer
	// because the synthetic Docker config still declares the model format as "gguf".
	paths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("GGUFPaths() error = %v", err)
	}

	// Should find two: original Docker format + raw ModelPack format
	if len(paths) != 2 {
		t.Errorf("Expected 2 GGUF paths, got %d", len(paths))
	}
}

// TestGGUFPaths_NoFalsePositive_SafetensorsModelPackType tests that a format-specific
// ModelPack safetensors layer is NOT incorrectly matched as GGUF, even when the model
// config declares the format as "gguf".
// Regression test for: IsModelPackWeightMediaType applying the format-agnostic fallback
// to format-specific types, causing cross-format false positives.
func TestGGUFPaths_NoFalsePositive_SafetensorsModelPackType(t *testing.T) {
	// Build a GGUF artifact but add an extra layer with the safetensors-specific ModelPack type.
	mdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(
			filepath.Join("..", "..", "assets", "dummy.gguf"),
			oci.MediaType("application/vnd.cncf.model.weight.v1.safetensors"),
		),
	)

	paths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("GGUFPaths() error = %v", err)
	}

	// Should find only the one Docker-format GGUF layer.
	// The safetensors-typed layer must NOT be returned as a GGUF path.
	if len(paths) != 1 {
		t.Errorf("Expected 1 GGUF path (safetensors layer must not match), got %d", len(paths))
	}
}

// TestSafetensorsPaths_NoFalsePositive_GGUFModelPackType tests that a format-specific
// ModelPack GGUF layer is NOT incorrectly matched as safetensors, even when the model
// config declares the format as "safetensors".
func TestSafetensorsPaths_NoFalsePositive_GGUFModelPackType(t *testing.T) {
	// Build a safetensors artifact but add an extra layer with the GGUF-specific ModelPack type.
	mdl := testutil.NewSafetensorsArtifact(
		t,
		filepath.Join("..", "..", "assets", "dummy.gguf"),
		testutil.Layer(
			filepath.Join("..", "..", "assets", "dummy.gguf"),
			oci.MediaType("application/vnd.cncf.model.weight.v1.gguf"),
		),
	)

	paths, err := partial.SafetensorsPaths(mdl)
	if err != nil {
		t.Fatalf("SafetensorsPaths() error = %v", err)
	}

	// Should find only the one Docker-format safetensors layer.
	// The GGUF-typed layer must NOT be returned as a safetensors path.
	if len(paths) != 1 {
		t.Errorf("Expected 1 safetensors path (GGUF layer must not match), got %d", len(paths))
	}
}

// TestGGUFPaths_ModelPackRawNoConfigFormat tests that GGUFPaths can find raw
// ModelPack weight layers even when the model config omits the format field.
// This exercises the annotation-based format discovery fallback introduced to
// handle CNCF ModelPack models that do not populate config.format.
func TestGGUFPaths_ModelPackRawNoConfigFormat(t *testing.T) {
	// Build a CNCF ModelPack artifact with an empty config format and a raw
	// weight layer whose filepath annotation ends in ".gguf".
	mdl := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{Config: modelpack.ModelConfig{}}, // format intentionally empty
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "model.gguf",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
		},
	)

	paths, err := partial.GGUFPaths(mdl)
	if err != nil {
		t.Fatalf("GGUFPaths() error = %v", err)
	}

	// Should discover one GGUF path via the ".gguf" extension fallback.
	if len(paths) != 1 {
		t.Errorf("Expected 1 GGUF path via annotation fallback, got %d", len(paths))
	}
}

// TestSafetensorsPaths_ModelPackRawNoConfigFormat mirrors the GGUF test above
// for the safetensors format.
func TestSafetensorsPaths_ModelPackRawNoConfigFormat(t *testing.T) {
	mdl := testutil.NewModelPackArtifact(
		t,
		modelpack.Model{Config: modelpack.ModelConfig{}}, // format intentionally empty
		testutil.LayerSpec{
			Path:         filepath.Join("..", "..", "assets", "dummy.gguf"),
			RelativePath: "model.safetensors",
			MediaType:    oci.MediaType(modelpack.MediaTypeWeightRaw),
		},
	)

	paths, err := partial.SafetensorsPaths(mdl)
	if err != nil {
		t.Fatalf("SafetensorsPaths() error = %v", err)
	}

	// Should discover one safetensors path via the ".safetensors" extension fallback.
	if len(paths) != 1 {
		t.Errorf("Expected 1 safetensors path via annotation fallback, got %d", len(paths))
	}
}
