package builder_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestBuilder(t *testing.T) {
	// Create a builder from a GGUF file
	b, err := builder.FromGGUF(filepath.Join("..", "assets", "dummy.gguf"))
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
	b, err := builder.FromGGUF(filepath.Join("..", "assets", "dummy.gguf"))
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
	b, err := builder.FromGGUF(filepath.Join("..", "assets", "dummy.gguf"))
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

	b = b.WithContextSize(4096)

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
	initialBuilder, err := builder.FromGGUF(filepath.Join("..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create initial builder from GGUF: %v", err)
	}

	// Add license to the initial model
	initialBuilder, err = initialBuilder.WithLicense(filepath.Join("..", "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license: %v", err)
	}

	// Set initial context size
	initialBuilder = initialBuilder.WithContextSize(2048)

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
	repackagedBuilder = repackagedBuilder.WithContextSize(4096)

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
	initialBuilder, err := builder.FromGGUF(filepath.Join("..", "assets", "dummy.gguf"))
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
	// Create a mock model that fails when Layers() is called
	mockModel := &mockFailingModel{}

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

var _ builder.Target = &fakeTarget{}

type fakeTarget struct {
	artifact types.ModelArtifact
}

func (ft *fakeTarget) Write(ctx context.Context, artifact types.ModelArtifact, writer io.Writer) error {
	ft.artifact = artifact
	return nil
}

// mockFailingModel is a mock that fails when Layers() is called
type mockFailingModel struct {
	types.ModelArtifact
}

func (m *mockFailingModel) Layers() ([]oci.Layer, error) {
	return nil, fmt.Errorf("simulated layers error")
}
