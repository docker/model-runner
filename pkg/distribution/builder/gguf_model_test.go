package builder_test

import (
	"encoding/json"
	"path"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestGGUFModel(t *testing.T) {
	mdl, err := buildGGUFModel()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	t.Run("TestConfig", func(t *testing.T) {
		cfgInterface, err := mdl.Config()
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}
		if cfgInterface.GetFormat() != types.FormatGGUF {
			t.Fatalf("Unexpected format: got %s expected %s", cfgInterface.GetFormat(), types.FormatGGUF)
		}
		if cfgInterface.GetParameters() != "183" {
			t.Fatalf("Unexpected parameters: got %s expected %s", cfgInterface.GetParameters(), "183")
		}
		if cfgInterface.GetArchitecture() != "llama" {
			t.Fatalf("Unexpected architecture: got %s expected %s", cfgInterface.GetArchitecture(), "llama")
		}
		if cfgInterface.GetQuantization() != "Unknown" {
			t.Fatalf("Unexpected quantization: got %s expected %s", cfgInterface.GetQuantization(), "Unknown")
		}
		if cfgInterface.GetSize() != "864B" {
			t.Fatalf("Unexpected size: got %s expected %s", cfgInterface.GetSize(), "864B")
		}

		// Test GGUF metadata (Docker format specific)
		cfg, ok := cfgInterface.(*types.Config)
		if !ok {
			t.Fatal("Expected *types.Config for GGUF model")
		}
		if cfg.GGUF == nil {
			t.Fatal("Expected GGUF metadata to be present")
		}
		// Verify expected metadata fields from the example
		expectedParams := map[string]string{
			"some.parameter.uint8":   "18",
			"some.parameter.int8":    "-19",
			"some.parameter.uint16":  "4660",
			"some.parameter.int16":   "-4661",
			"some.parameter.uint32":  "305419896",
			"some.parameter.int32":   "-305419897",
			"some.parameter.float32": "0.123457",
			"some.parameter.uint64":  "1311768467463790320",
			"some.parameter.int64":   "-1311768467463790321",
			"some.parameter.float64": "0.123457",
			"some.parameter.bool":    "true",
			"some.parameter.string":  "hello world",
			"some.parameter.arr.i16": "1, 2, 3, 4",
		}

		for key, expectedValue := range expectedParams {
			actualValue, ok := cfg.GGUF[key]
			if !ok {
				t.Errorf("Expected key '%s' in GGUF metadata", key)
				continue
			}
			if actualValue != expectedValue {
				t.Errorf("For key '%s': expected value '%s', got '%s'", key, expectedValue, actualValue)
			}
		}
	})

	t.Run("TestDescriptor", func(t *testing.T) {
		desc, err := mdl.Descriptor()
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}
		if desc.Created == nil {
			t.Fatal("Expected created time to be set: got nil")
		}
	})

	t.Run("TestManifest", func(t *testing.T) {
		manifest, err := mdl.Manifest()
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}
		if len(manifest.Layers) != 1 {
			t.Fatalf("Expected 1 layer, got %d", len(manifest.Layers))
		}
		if manifest.Layers[0].MediaType != types.MediaTypeGGUF {
			t.Fatalf("Expected layer with media type %s, got %s", types.MediaTypeGGUF, manifest.Layers[0].MediaType)
		}
	})

	t.Run("TestAnnotations", func(t *testing.T) {
		manifest, err := mdl.Manifest()
		if err != nil {
			t.Fatalf("Failed to get manifest: %v", err)
		}
		if len(manifest.Layers) != 1 {
			t.Fatalf("Expected 1 layer, got %d", len(manifest.Layers))
		}

		layer := manifest.Layers[0]
		if layer.Annotations == nil {
			t.Fatal("Expected annotations to be present")
		}

		filePath, ok := layer.Annotations[types.AnnotationFilePath]
		if !ok {
			t.Errorf("Expected annotation %s to be present", types.AnnotationFilePath)
		}

		if filePath != path.Base("dummy.gguf") {
			t.Errorf("Expected file path annotation to be '%s', got '%s'", path.Base("dummy.gguf"), filePath)
		}

		if _, ok := layer.Annotations[types.AnnotationFileMetadata]; !ok {
			t.Errorf("Expected annotation %s to be present", types.AnnotationFileMetadata)
		}

		if val, ok := layer.Annotations[types.AnnotationMediaTypeUntested]; !ok {
			t.Errorf("Expected annotation %s to be present", types.AnnotationMediaTypeUntested)
		} else if val != "false" {
			t.Errorf("Expected annotation %s to be 'false', got '%s'", types.AnnotationMediaTypeUntested, val)
		}

		metadataJSON := layer.Annotations[types.AnnotationFileMetadata]
		var metadata types.FileMetadata
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			t.Fatalf("Failed to unmarshal file metadata: %v", err)
		}

		if metadata.Name != "dummy.gguf" {
			t.Errorf("Expected file name 'dummy.gguf', got '%s'", metadata.Name)
		}
		if metadata.Size == 0 {
			t.Error("Expected file size to be non-zero")
		}
		if metadata.Typeflag != 0 {
			t.Errorf("Expected Typeflag 0 for regular file, got %d", metadata.Typeflag)
		}
		if metadata.Mode == 0 {
			t.Error("Expected file mode to be non-zero")
		}
		if metadata.ModTime.IsZero() {
			t.Error("Expected modification time to be set")
		}
		if metadata.Uid != 0 {
			t.Error("Expected Uid to be set with default 0")
		}
	})
}

func buildGGUFModel() (types.ModelArtifact, error) {
	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		return nil, err
	}
	return b.Model(), nil
}
