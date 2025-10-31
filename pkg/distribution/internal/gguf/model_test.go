package gguf_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestGGUF(t *testing.T) {
	t.Run("TestGGUFModel", func(t *testing.T) {
		mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		t.Run("TestConfig", func(t *testing.T) {
			cfg, err := mdl.Config()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if cfg.Format != types.FormatGGUF {
				t.Fatalf("Unexpected format: got %s expected %s", cfg.Format, types.FormatGGUF)
			}
			if cfg.Parameters != "183.00" {
				t.Fatalf("Unexpected parameters: got %s expected %s", cfg.Parameters, "183.00")
			}
			if cfg.Architecture != "llama" {
				t.Fatalf("Unexpected architecture: got %s expected %s", cfg.Parameters, "llama")
			}
			if cfg.Quantization != "Unknown" { // todo: testdata with a real value
				t.Fatalf("Unexpected quantization: got %s expected %s", cfg.Quantization, "Unknown")
			}
			if cfg.Size != "864B" {
				t.Fatalf("Unexpected size: got %s expected %s", cfg.Size, "864B")
			}

			// Test GGUF metadata
			if cfg.GGUF == nil {
				t.Fatal("Expected GGUF metadata to be present")
			}
			// Verify all expected metadata fields from the example https://github.com/ggml-org/llama.cpp/blob/44cd8d91ff2c9e4a0f2e3151f8d6f04c928e2571/examples/gguf/gguf.cpp#L24
			expectedParams := map[string]string{
				"some.parameter.uint8":   "18",                   // 0x12
				"some.parameter.int8":    "-19",                  // -0x13
				"some.parameter.uint16":  "4660",                 // 0x1234
				"some.parameter.int16":   "-4661",                // -0x1235
				"some.parameter.uint32":  "305419896",            // 0x12345678
				"some.parameter.int32":   "-305419897",           // -0x12345679
				"some.parameter.float32": "0.123457",             // 0.123456789f
				"some.parameter.uint64":  "1311768467463790320",  // 0x123456789abcdef0
				"some.parameter.int64":   "-1311768467463790321", // -0x123456789abcdef1
				"some.parameter.float64": "0.123457",             // 0.1234567890123456789
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
				t.Fatal("Expected created time to be set: got ni")
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
	})
}

func TestGGUFShards(t *testing.T) {
	t.Run("TestShardedGGUFModel", func(t *testing.T) {
		mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy-00001-of-00002.gguf"))
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		t.Run("TestConfig", func(t *testing.T) {
			cfg, err := mdl.Config()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if cfg.Format != types.FormatGGUF {
				t.Fatalf("Unexpected format: got %s expected %s", cfg.Format, types.FormatGGUF)
			}
			if cfg.Parameters != "183.00" {
				t.Fatalf("Unexpected parameters: got %s expected %s", cfg.Parameters, "183.00")
			}
			if cfg.Architecture != "llama" {
				t.Fatalf("Unexpected architecture: got %s expected %s", cfg.Parameters, "llama")
			}
			if cfg.Quantization != "Unknown" { // todo: testdata with a real value
				t.Fatalf("Unexpected quantization: got %s expected %s", cfg.Quantization, "Unknown")
			}
			if cfg.Size != "864B" {
				t.Fatalf("Unexpected size: got %s expected %s", cfg.Size, "864B")
			}

			// Test GGUF metadata
			if cfg.GGUF == nil {
				t.Fatal("Expected GGUF metadata to be present")
			}
			// Verify all expected metadata fields from the example https://github.com/ggml-org/llama.cpp/blob/44cd8d91ff2c9e4a0f2e3151f8d6f04c928e2571/examples/gguf/gguf.cpp#L24
			expectedParams := map[string]string{
				"some.parameter.uint8":   "18",                   // 0x12
				"some.parameter.int8":    "-19",                  // -0x13
				"some.parameter.uint16":  "4660",                 // 0x1234
				"some.parameter.int16":   "-4661",                // -0x1235
				"some.parameter.uint32":  "305419896",            // 0x12345678
				"some.parameter.int32":   "-305419897",           // -0x12345679
				"some.parameter.float32": "0.123457",             // 0.123456789f
				"some.parameter.uint64":  "1311768467463790320",  // 0x123456789abcdef0
				"some.parameter.int64":   "-1311768467463790321", // -0x123456789abcdef1
				"some.parameter.float64": "0.123457",             // 0.1234567890123456789
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
				t.Fatal("Expected created time to be set: got ni")
			}
		})

		t.Run("TestManifest", func(t *testing.T) {
			manifest, err := mdl.Manifest()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if len(manifest.Layers) != 2 {
				t.Fatalf("Expected 2 layers, got %d", len(manifest.Layers))
			}
			if manifest.Layers[0].MediaType != types.MediaTypeGGUF {
				t.Fatalf("Expected layer with media type %s, got %s", types.MediaTypeGGUF, manifest.Layers[0].MediaType)
			}
		})
	})
}
