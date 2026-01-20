package distribution

import (
	"os"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
)

func TestECRIntegration(t *testing.T) {
	// Skip if ECR integration is not enabled
	if os.Getenv("TEST_ECR_ENABLED") != "true" {
		t.Skip("Skipping ECR integration test")
	}

	// Get ECR tag from environment
	ecrTag := os.Getenv("TEST_ECR_TAG")
	if ecrTag == "" {
		t.Fatal("TEST_ECR_TAG environment variable is required")
	}

	tempDir := t.TempDir()

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Read test model file
	modelFile := "../assets/dummy.gguf"
	modelContent, err := os.ReadFile(modelFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	t.Run("Push", func(t *testing.T) {
		b, err := builder.FromPath(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}
		mdl := b.Model()
		if err := client.store.Write(mdl, []string{ecrTag}, nil); err != nil {
			t.Fatalf("Failed to write model to store: %v", err)
		}
		if err := client.PushModel(t.Context(), ecrTag, nil); err != nil {
			t.Fatalf("Failed to push model to ECR: %v", err)
		}
		if _, err := client.DeleteModel(ecrTag, false); err != nil { // cleanup
			t.Fatalf("Failed to delete model from store: %v", err)
		}
	})

	// Test pull from ECR
	t.Run("Pull without progress", func(t *testing.T) {
		err := client.PullModel(t.Context(), ecrTag, nil)
		if err != nil {
			t.Fatalf("Failed to pull model from ECR: %v", err)
		}

		model, err := client.GetModel(ecrTag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		modelPaths, err := model.GGUFPaths()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}
		if len(modelPaths) != 1 {
			t.Fatalf("Unexpected number of model files: %d", len(modelPaths))
		}
		modelPath := modelPaths[0]
		defer os.Remove(modelPath)

		// Verify model content
		pulledContent, err := os.ReadFile(modelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	// Test get model info
	t.Run("GetModel", func(t *testing.T) {
		model, err := client.GetModel(ecrTag)
		if err != nil {
			t.Fatalf("Failed to get model info: %v", err)
		}

		if len(model.Tags()) == 0 || model.Tags()[0] != ecrTag {
			t.Errorf("Model tags don't match: got %v, want [%s]", model.Tags(), ecrTag)
		}
	})
}
