package builder_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// TestWithCreatedDeterministicDigest verifies that using WithCreated produces
// deterministic digests: the same file + same timestamp should always yield
// the same manifest digest, while different timestamps yield different digests.
func TestWithCreatedDeterministicDigest(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	// Build twice with the same fixed timestamp
	b1, err := builder.FromPath(ggufPath, builder.WithCreated(fixedTime))
	if err != nil {
		t.Fatalf("FromPath (first) failed: %v", err)
	}
	b2, err := builder.FromPath(ggufPath, builder.WithCreated(fixedTime))
	if err != nil {
		t.Fatalf("FromPath (second) failed: %v", err)
	}

	target1 := &fakeTarget{}
	target2 := &fakeTarget{}
	if err := b1.Build(t.Context(), target1, nil); err != nil {
		t.Fatalf("Build (first) failed: %v", err)
	}
	if err := b2.Build(t.Context(), target2, nil); err != nil {
		t.Fatalf("Build (second) failed: %v", err)
	}

	digest1, err := target1.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (first) failed: %v", err)
	}
	digest2, err := target2.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (second) failed: %v", err)
	}

	if digest1 != digest2 {
		t.Errorf("Expected identical digests with same timestamp, got %v and %v", digest1, digest2)
	}

	// Build with a different timestamp and verify digest differs
	differentTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	b3, err := builder.FromPath(ggufPath, builder.WithCreated(differentTime))
	if err != nil {
		t.Fatalf("FromPath (third) failed: %v", err)
	}
	target3 := &fakeTarget{}
	if err := b3.Build(t.Context(), target3, nil); err != nil {
		t.Fatalf("Build (third) failed: %v", err)
	}
	digest3, err := target3.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (third) failed: %v", err)
	}

	if digest1 == digest3 {
		t.Errorf("Expected different digests with different timestamps, but both were %v", digest1)
	}
}

// TestWithCreatedFromPaths verifies that WithCreated works with FromPaths as well.
func TestWithCreatedFromPaths(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	b1, err := builder.FromPaths([]string{ggufPath}, builder.WithCreated(fixedTime))
	if err != nil {
		t.Fatalf("FromPaths (first) failed: %v", err)
	}
	b2, err := builder.FromPaths([]string{ggufPath}, builder.WithCreated(fixedTime))
	if err != nil {
		t.Fatalf("FromPaths (second) failed: %v", err)
	}

	target1 := &fakeTarget{}
	target2 := &fakeTarget{}
	if err := b1.Build(t.Context(), target1, nil); err != nil {
		t.Fatalf("Build (first) failed: %v", err)
	}
	if err := b2.Build(t.Context(), target2, nil); err != nil {
		t.Fatalf("Build (second) failed: %v", err)
	}

	digest1, err := target1.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (first) failed: %v", err)
	}
	digest2, err := target2.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (second) failed: %v", err)
	}

	if digest1 != digest2 {
		t.Errorf("Expected identical digests with same timestamp, got %v and %v", digest1, digest2)
	}
}

func TestBuilder(t *testing.T) {
	// Create a builder from a GGUF file
	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create builder from GGUF: %v", err)
	}

	// Add multimodal projector
	b, err = b.WithMultimodalProjector(filepath.Join("..", "assets", "dummy.mmproj"))
	if err != nil {
		t.Fatalf("Failed to add multimodal projector: %v", err)
	}

	// Add a chat template file
	b, err = b.WithChatTemplateFile(filepath.Join("..", "assets", "template.jinja"))
	if err != nil {
		t.Fatalf("Failed to add multimodal projector: %v", err)
	}

	// Build the model
	target := &fakeTarget{}
	if err := b.Build(t.Context(), target, nil); err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}

	// Verify the model has the expected layers
	manifest, err := target.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}

	// Should have 3 layers: GGUF + multimodal projector + chat template
	if len(manifest.Layers) != 3 {
		t.Fatalf("Expected 2 layers, got %d", len(manifest.Layers))
	}

	// Check that each layer has the expected
	if manifest.Layers[0].MediaType != types.MediaTypeGGUF {
		t.Fatalf("Expected first layer with media type %s, got %s", types.MediaTypeGGUF, manifest.Layers[0].MediaType)
	}
	if manifest.Layers[1].MediaType != types.MediaTypeMultimodalProjector {
		t.Fatalf("Expected first layer with media type %s, got %s", types.MediaTypeMultimodalProjector, manifest.Layers[1].MediaType)
	}
	if manifest.Layers[2].MediaType != types.MediaTypeChatTemplate {
		t.Fatalf("Expected first layer with media type %s, got %s", types.MediaTypeChatTemplate, manifest.Layers[2].MediaType)
	}
}

func TestWithMultimodalProjectorInvalidPath(t *testing.T) {
	// Create a builder from a GGUF file
	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create builder from GGUF: %v", err)
	}

	// Try to add multimodal projector with invalid path
	_, err = b.WithMultimodalProjector("nonexistent/path/to/mmproj")
	if err == nil {
		t.Error("Expected error when adding multimodal projector with invalid path")
	}
}

func TestWithMultimodalProjectorChaining(t *testing.T) {
	// Create a builder from a GGUF file
	b, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create builder from GGUF: %v", err)
	}

	// Chain multiple operations: license + multimodal projector + context size
	b, err = b.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license: %v", err)
	}

	b, err = b.WithMultimodalProjector(filepath.Join("..", "assets", "dummy.mmproj"))
	if err != nil {
		t.Fatalf("Failed to add multimodal projector: %v", err)
	}

	b, err = b.WithContextSize(4096)
	if err != nil {
		t.Fatalf("Failed to set context size: %v", err)
	}

	// Build the model
	target := &fakeTarget{}
	if err := b.Build(t.Context(), target, nil); err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}

	// Verify the final model has all expected layers and properties
	manifest, err := target.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}

	// Should have 3 layers: GGUF + license + multimodal projector
	if len(manifest.Layers) != 3 {
		t.Fatalf("Expected 3 layers, got %d", len(manifest.Layers))
	}

	// Check media types - using string comparison since we can't use types.MediaType directly
	expectedMediaTypes := map[string]bool{
		string(types.MediaTypeGGUF):                false,
		string(types.MediaTypeLicense):             false,
		string(types.MediaTypeMultimodalProjector): false,
	}

	for _, layer := range manifest.Layers {
		if _, exists := expectedMediaTypes[string(layer.MediaType)]; exists {
			expectedMediaTypes[string(layer.MediaType)] = true
		}
	}

	for mediaType, found := range expectedMediaTypes {
		if !found {
			t.Errorf("Expected to find layer with media type %s", mediaType)
		}
	}

	// Check context size
	config, err := target.artifact.Config()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	if config.GetContextSize() == nil || *config.GetContextSize() != 4096 {
		t.Errorf("Expected context size 4096, got %v", config.GetContextSize())
	}

	// Note: We can't directly test GGUFPath() and MMPROJPath() on ModelArtifact interface
	// but we can verify the layers were added with correct media types above
}

func TestFromModel(t *testing.T) {
	// Step 1: Create an initial model from GGUF with context size 2048
	initialBuilder, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create initial builder from GGUF: %v", err)
	}

	// Add license to the initial model
	initialBuilder, err = initialBuilder.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license: %v", err)
	}

	// Set initial context size
	initialBuilder, err = initialBuilder.WithContextSize(2048)
	if err != nil {
		t.Fatalf("Failed to set context size: %v", err)
	}

	// Build the initial model
	initialTarget := &fakeTarget{}
	if err := initialBuilder.Build(t.Context(), initialTarget, nil); err != nil {
		t.Fatalf("Failed to build initial model: %v", err)
	}

	// Verify initial model properties
	initialConfig, err := initialTarget.artifact.Config()
	if err != nil {
		t.Fatalf("Failed to get initial config: %v", err)
	}
	if initialConfig.GetContextSize() == nil || *initialConfig.GetContextSize() != 2048 {
		t.Fatalf("Expected initial context size 2048, got %v", initialConfig.GetContextSize())
	}

	// Step 2: Use FromModel() to create a new builder from the existing model
	repackagedBuilder, err := builder.FromModel(initialTarget.artifact)
	if err != nil {
		t.Fatalf("Failed to create builder from model: %v", err)
	}

	// Step 3: Modify the context size to 4096
	repackagedBuilder, err = repackagedBuilder.WithContextSize(4096)
	if err != nil {
		t.Fatalf("Failed to set context size: %v", err)
	}

	// Step 4: Build the repackaged model
	repackagedTarget := &fakeTarget{}
	if err := repackagedBuilder.Build(t.Context(), repackagedTarget, nil); err != nil {
		t.Fatalf("Failed to build repackaged model: %v", err)
	}

	// Step 5: Verify the repackaged model has the new context size
	repackagedConfig, err := repackagedTarget.artifact.Config()
	if err != nil {
		t.Fatalf("Failed to get repackaged config: %v", err)
	}

	if repackagedConfig.GetContextSize() == nil || *repackagedConfig.GetContextSize() != 4096 {
		t.Errorf("Expected repackaged context size 4096, got %v", repackagedConfig.GetContextSize())
	}

	// Step 6: Verify the original layers are preserved
	initialManifest, err := initialTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get initial manifest: %v", err)
	}

	repackagedManifest, err := repackagedTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get repackaged manifest: %v", err)
	}

	// Should have the same number of layers (GGUF + license)
	if len(repackagedManifest.Layers) != len(initialManifest.Layers) {
		t.Errorf("Expected %d layers in repackaged model, got %d", len(initialManifest.Layers), len(repackagedManifest.Layers))
	}

	// Verify layer media types are preserved
	for i, initialLayer := range initialManifest.Layers {
		if i >= len(repackagedManifest.Layers) {
			break
		}
		if initialLayer.MediaType != repackagedManifest.Layers[i].MediaType {
			t.Errorf("Layer %d media type mismatch: expected %s, got %s", i, initialLayer.MediaType, repackagedManifest.Layers[i].MediaType)
		}
	}
}

func TestFromModelWithAdditionalLayers(t *testing.T) {
	// Create an initial model from GGUF
	initialBuilder, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create initial builder from GGUF: %v", err)
	}

	// Build the initial model
	initialTarget := &fakeTarget{}
	if err := initialBuilder.Build(t.Context(), initialTarget, nil); err != nil {
		t.Fatalf("Failed to build initial model: %v", err)
	}

	// Use FromModel() and add additional layers
	repackagedBuilder, err := builder.FromModel(initialTarget.artifact)
	if err != nil {
		t.Fatalf("Failed to create builder from model: %v", err)
	}
	repackagedBuilder, err = repackagedBuilder.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license to repackaged model: %v", err)
	}

	repackagedBuilder, err = repackagedBuilder.WithMultimodalProjector(filepath.Join("..", "assets", "dummy.mmproj"))
	if err != nil {
		t.Fatalf("Failed to add multimodal projector to repackaged model: %v", err)
	}

	// Build the repackaged model
	repackagedTarget := &fakeTarget{}
	if err := repackagedBuilder.Build(t.Context(), repackagedTarget, nil); err != nil {
		t.Fatalf("Failed to build repackaged model: %v", err)
	}

	// Verify the repackaged model has all layers
	initialManifest, err := initialTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get initial manifest: %v", err)
	}

	repackagedManifest, err := repackagedTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get repackaged manifest: %v", err)
	}

	// Should have original layers plus license and mmproj (2 additional layers)
	expectedLayers := len(initialManifest.Layers) + 2
	if len(repackagedManifest.Layers) != expectedLayers {
		t.Errorf("Expected %d layers in repackaged model, got %d", expectedLayers, len(repackagedManifest.Layers))
	}

	// Verify the new layers were added
	hasLicense := false
	hasMMProj := false
	for _, layer := range repackagedManifest.Layers {
		if layer.MediaType == types.MediaTypeLicense {
			hasLicense = true
		}
		if layer.MediaType == types.MediaTypeMultimodalProjector {
			hasMMProj = true
		}
	}

	if !hasLicense {
		t.Error("Expected repackaged model to have license layer")
	}
	if !hasMMProj {
		t.Error("Expected repackaged model to have multimodal projector layer")
	}
}

// TestFromModelErrorHandling tests that FromModel properly handles and surfaces errors from mdl.Layers()
func TestFromModelErrorHandling(t *testing.T) {
	mockModel := testutil.WithLayersError(testutil.NewGGUFArtifact(t, filepath.Join("..", "assets", "dummy.gguf")), fmt.Errorf("simulated layers error"))

	// Attempt to create a builder from the failing model
	_, err := builder.FromModel(mockModel)
	if err == nil {
		t.Fatal("Expected error when model.Layers() fails, got nil")
	}

	// Verify the error message indicates the issue
	expectedErrMsg := "getting model layers"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedErrMsg, err)
	}
}

// TestFromPathCNCFFormat verifies that FromPath with WithFormat(BuildFormatCNCF) produces
// a valid CNCF ModelPack artifact with correct media types, artifact type, and config.
func TestFromPathCNCFFormat(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	b, err := builder.FromPath(ggufPath,
		builder.WithFormat(builder.BuildFormatCNCF),
		builder.WithCreated(fixedTime),
	)
	if err != nil {
		t.Fatalf("FromPath with CNCF format failed: %v", err)
	}

	target := &fakeTarget{}
	if err := b.Build(t.Context(), target, nil); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 1. Verify manifest has CNCF artifact type.
	manifest, err := target.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}
	if manifest.ArtifactType != modelpack.ArtifactTypeModelManifest {
		t.Errorf("Expected artifactType %q, got %q",
			modelpack.ArtifactTypeModelManifest, manifest.ArtifactType)
	}

	// 2. Verify config media type is CNCF model config.
	if manifest.Config.MediaType != oci.MediaType(modelpack.MediaTypeModelConfigV1) {
		t.Errorf("Expected config media type %q, got %q",
			modelpack.MediaTypeModelConfigV1, manifest.Config.MediaType)
	}

	// 3. Verify all layers have CNCF media types (not Docker media types).
	for i, layer := range manifest.Layers {
		mt := string(layer.MediaType)
		if !strings.HasPrefix(mt, modelpack.MediaTypePrefix) {
			t.Errorf("Layer %d has non-CNCF media type %q (expected prefix %q)",
				i, mt, modelpack.MediaTypePrefix)
		}
	}

	// 4. Verify the weight layer specifically uses the CNCF weight media type.
	if len(manifest.Layers) == 0 {
		t.Fatal("Expected at least one layer")
	}
	weightMT := manifest.Layers[0].MediaType
	if weightMT != oci.MediaType(modelpack.MediaTypeWeightRaw) {
		t.Errorf("Expected weight layer media type %q, got %q",
			modelpack.MediaTypeWeightRaw, weightMT)
	}

	// 5. Verify the raw config is valid ModelPack JSON with correct fields.
	rawCfg, err := target.artifact.RawConfigFile()
	if err != nil {
		t.Fatalf("Failed to get raw config: %v", err)
	}
	var mp modelpack.Model
	if err := json.Unmarshal(rawCfg, &mp); err != nil {
		t.Fatalf("Failed to unmarshal CNCF config: %v", err)
	}
	if mp.Config.Format != "gguf" {
		t.Errorf("Expected config.format %q, got %q", "gguf", mp.Config.Format)
	}
	if mp.ModelFS.Type != "layers" {
		t.Errorf("Expected modelfs.type %q, got %q", "layers", mp.ModelFS.Type)
	}
	if len(mp.ModelFS.DiffIDs) == 0 {
		t.Error("Expected at least one diffId in modelfs")
	}
	if mp.Descriptor.CreatedAt == nil {
		t.Error("Expected descriptor.createdAt to be set")
	} else if !mp.Descriptor.CreatedAt.Equal(fixedTime) {
		t.Errorf("Expected descriptor.createdAt %v, got %v", fixedTime, *mp.Descriptor.CreatedAt)
	}

	// 6. Verify the JSON tags are camelCase (spec-compliant).
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(rawCfg, &rawMap); err != nil {
		t.Fatalf("Failed to unmarshal config to map: %v", err)
	}
	// Must have "modelfs" (not "model_fs").
	if _, ok := rawMap["modelfs"]; !ok {
		t.Error("Config JSON missing 'modelfs' key")
	}
	// Verify modelfs contains "diffIds" (camelCase, not "diff_ids").
	if modelfsRaw, ok := rawMap["modelfs"]; ok {
		var modelfsMap map[string]json.RawMessage
		if err := json.Unmarshal(modelfsRaw, &modelfsMap); err != nil {
			t.Fatalf("Failed to unmarshal modelfs: %v", err)
		}
		if _, ok := modelfsMap["diffIds"]; !ok {
			t.Error("modelfs JSON missing 'diffIds' key (expected camelCase)")
		}
		if _, ok := modelfsMap["diff_ids"]; ok {
			t.Error("modelfs JSON has 'diff_ids' (snake_case) — should be 'diffIds' (camelCase)")
		}
	}
	// Verify config contains "paramSize" (not "param_size").
	if configRaw, ok := rawMap["config"]; ok {
		var configMap map[string]json.RawMessage
		if err := json.Unmarshal(configRaw, &configMap); err != nil {
			t.Fatalf("Failed to unmarshal config section: %v", err)
		}
		if _, ok := configMap["param_size"]; ok {
			t.Error("config JSON has 'param_size' (snake_case) — should be 'paramSize' (camelCase)")
		}
	}
}

// TestFromPathCNCFWithAdditionalLayers verifies that additional layers added
// to a CNCF builder get CNCF media types, not Docker media types.
func TestFromPathCNCFWithAdditionalLayers(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")

	b, err := builder.FromPath(ggufPath, builder.WithFormat(builder.BuildFormatCNCF))
	if err != nil {
		t.Fatalf("FromPath failed: %v", err)
	}

	// Add license
	b, err = b.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license: %v", err)
	}

	// Add multimodal projector
	b, err = b.WithMultimodalProjector(filepath.Join("..", "assets", "dummy.mmproj"))
	if err != nil {
		t.Fatalf("Failed to add multimodal projector: %v", err)
	}

	// Add chat template
	b, err = b.WithChatTemplateFile(filepath.Join("..", "assets", "template.jinja"))
	if err != nil {
		t.Fatalf("Failed to add chat template: %v", err)
	}

	target := &fakeTarget{}
	if err := b.Build(t.Context(), target, nil); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	manifest, err := target.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}

	// Should have 4 layers: weight + license + mmproj + chat template
	if len(manifest.Layers) != 4 {
		t.Fatalf("Expected 4 layers, got %d", len(manifest.Layers))
	}

	// ALL layers must have CNCF media type prefix.
	for i, layer := range manifest.Layers {
		mt := string(layer.MediaType)
		if !strings.HasPrefix(mt, modelpack.MediaTypePrefix) {
			t.Errorf("Layer %d has non-CNCF media type %q", i, mt)
		}
	}

	// No Docker media types should appear.
	dockerMTs := []oci.MediaType{
		types.MediaTypeGGUF,
		types.MediaTypeLicense,
		types.MediaTypeMultimodalProjector,
		types.MediaTypeChatTemplate,
	}
	for _, layer := range manifest.Layers {
		for _, dmt := range dockerMTs {
			if layer.MediaType == dmt {
				t.Errorf("Found Docker media type %q in CNCF artifact", dmt)
			}
		}
	}
}

// TestFromPathCNCFContextSizeError verifies that WithContextSize returns an error
// when the output format is CNCF (context size is not in the CNCF spec).
func TestFromPathCNCFContextSizeError(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")

	b, err := builder.FromPath(ggufPath, builder.WithFormat(builder.BuildFormatCNCF))
	if err != nil {
		t.Fatalf("FromPath failed: %v", err)
	}

	_, err = b.WithContextSize(4096)
	if err == nil {
		t.Fatal("Expected error when setting context size with CNCF format, got nil")
	}
	if !strings.Contains(err.Error(), "--context-size is not supported") {
		t.Errorf("Expected error about context-size not supported, got: %v", err)
	}
}

// TestFromModelToCNCF verifies that FromModel with WithFormat(BuildFormatCNCF) correctly
// converts a Docker-format model to CNCF ModelPack format.
func TestFromModelToCNCF(t *testing.T) {
	// Step 1: Create a Docker-format model with a license layer.
	dockerBuilder, err := builder.FromPath(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("FromPath failed: %v", err)
	}
	dockerBuilder, err = dockerBuilder.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("WithLicense failed: %v", err)
	}

	dockerTarget := &fakeTarget{}
	if err := dockerBuilder.Build(t.Context(), dockerTarget, nil); err != nil {
		t.Fatalf("Build Docker model failed: %v", err)
	}

	// Verify the Docker model has Docker media types.
	dockerManifest, err := dockerTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get Docker manifest: %v", err)
	}
	for _, layer := range dockerManifest.Layers {
		if strings.HasPrefix(string(layer.MediaType), modelpack.MediaTypePrefix) {
			t.Fatalf("Docker model should not have CNCF media types, found %q", layer.MediaType)
		}
	}

	// Step 2: Convert Docker model to CNCF format.
	cncfBuilder, err := builder.FromModel(dockerTarget.artifact, builder.WithFormat(builder.BuildFormatCNCF))
	if err != nil {
		t.Fatalf("FromModel with CNCF format failed: %v", err)
	}

	cncfTarget := &fakeTarget{}
	if err := cncfBuilder.Build(t.Context(), cncfTarget, nil); err != nil {
		t.Fatalf("Build CNCF model failed: %v", err)
	}

	// Step 3: Verify the CNCF model.
	cncfManifest, err := cncfTarget.artifact.Manifest()
	if err != nil {
		t.Fatalf("Failed to get CNCF manifest: %v", err)
	}

	// Artifact type must be set.
	if cncfManifest.ArtifactType != modelpack.ArtifactTypeModelManifest {
		t.Errorf("Expected artifactType %q, got %q",
			modelpack.ArtifactTypeModelManifest, cncfManifest.ArtifactType)
	}

	// Config media type must be CNCF.
	if cncfManifest.Config.MediaType != oci.MediaType(modelpack.MediaTypeModelConfigV1) {
		t.Errorf("Expected config media type %q, got %q",
			modelpack.MediaTypeModelConfigV1, cncfManifest.Config.MediaType)
	}

	// Same number of layers must be preserved.
	if len(cncfManifest.Layers) != len(dockerManifest.Layers) {
		t.Fatalf("Expected %d layers, got %d", len(dockerManifest.Layers), len(cncfManifest.Layers))
	}

	// All layers must have CNCF media types.
	for i, layer := range cncfManifest.Layers {
		mt := string(layer.MediaType)
		if !strings.HasPrefix(mt, modelpack.MediaTypePrefix) {
			t.Errorf("Layer %d has non-CNCF media type %q after conversion", i, mt)
		}
	}

	// Layer digests should be preserved (same content, different media type).
	for i := range dockerManifest.Layers {
		if dockerManifest.Layers[i].Digest != cncfManifest.Layers[i].Digest {
			t.Errorf("Layer %d digest changed after conversion: %v → %v",
				i, dockerManifest.Layers[i].Digest, cncfManifest.Layers[i].Digest)
		}
	}

	// Config should have the model architecture and format.
	cfg, err := cncfTarget.artifact.Config()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}
	if cfg.GetFormat() != types.FormatGGUF {
		t.Errorf("Expected format %q, got %q", types.FormatGGUF, cfg.GetFormat())
	}
}

// TestFromPathCNCFDeterministicDigest verifies that CNCF format builds
// with the same inputs produce the same digests.
func TestFromPathCNCFDeterministicDigest(t *testing.T) {
	ggufPath := filepath.Join("..", "assets", "dummy.gguf")
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	b1, err := builder.FromPath(ggufPath,
		builder.WithFormat(builder.BuildFormatCNCF),
		builder.WithCreated(fixedTime),
	)
	if err != nil {
		t.Fatalf("FromPath (first) failed: %v", err)
	}
	b2, err := builder.FromPath(ggufPath,
		builder.WithFormat(builder.BuildFormatCNCF),
		builder.WithCreated(fixedTime),
	)
	if err != nil {
		t.Fatalf("FromPath (second) failed: %v", err)
	}

	target1 := &fakeTarget{}
	target2 := &fakeTarget{}
	if err := b1.Build(t.Context(), target1, nil); err != nil {
		t.Fatalf("Build (first) failed: %v", err)
	}
	if err := b2.Build(t.Context(), target2, nil); err != nil {
		t.Fatalf("Build (second) failed: %v", err)
	}

	digest1, err := target1.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (first) failed: %v", err)
	}
	digest2, err := target2.artifact.Digest()
	if err != nil {
		t.Fatalf("Digest (second) failed: %v", err)
	}
	if digest1 != digest2 {
		t.Errorf("Expected identical digests for CNCF format with same inputs, got %v and %v", digest1, digest2)
	}
}

var _ builder.Target = &fakeTarget{}

type fakeTarget struct {
	artifact types.ModelArtifact
}

func (ft *fakeTarget) Write(ctx context.Context, artifact types.ModelArtifact, writer io.Writer) error {
	ft.artifact = artifact
	return nil
}
