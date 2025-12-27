package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/internal/store"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// TestStoreAPI tests the store API directly
func TestStoreAPI(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "store-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	storePath := filepath.Join(tempDir, "api-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	// Everything must handle directory deletion
	if err := os.RemoveAll(storePath); err != nil {
		t.Fatalf("Failed to remove store directory: %v", err)
	}

	model := newTestModel(t)
	layers, err := model.Layers()
	if err != nil {
		t.Fatalf("Failed to get layers: %v", err)
	}
	ggufDiffID, err := layers[0].DiffID()
	if err != nil {
		t.Fatalf("Failed to get diff ID: %v", err)
	}
	expectedBlobHash := ggufDiffID.String()

	digest, err := model.Digest()
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	if err := s.Write(model, []string{"api-model:latest"}, nil); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	t.Run("ReadByTag", func(t *testing.T) {
		mdl2, err := s.Read("api-model:latest")
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		readDigest, err := mdl2.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if digest != readDigest {
			t.Fatalf("Digest mismatch %s != %s", digest.Hex, readDigest.Hex)
		}
	})

	t.Run("ReadByID", func(t *testing.T) {
		id, err := model.ID()
		if err != nil {
			t.Fatalf("ID failed: %v", err)
		}
		mdl2, err := s.Read(id)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		readDigest, err := mdl2.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if digest != readDigest {
			t.Fatalf("Digest mismatch %s != %s", digest.Hex, readDigest.Hex)
		}
		if !containsTag(mdl2.Tags(), "api-model:latest") {
			t.Errorf("Expected tag api-model:latest, got %v", mdl2.Tags())
		}

	})

	t.Run("ReadNotFound", func(t *testing.T) {
		if _, err := s.Read("non-existent-model:latest"); !errors.Is(err, store.ErrModelNotFound) {
			t.Fatalf("Expected ErrModelNotFound got: %v", err)
		}
	})

	// Test List
	t.Run("List", func(t *testing.T) {
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("Expected 1 model, got %d", len(models))
		}
		if !containsTag(models[0].Tags, "api-model:latest") {
			t.Errorf("Expected tag api-model:latest, got %v", models[0].Tags)
		}
		if len(models[0].Files) != 3 {
			t.Fatalf("Expected 3 files (gguf, license, config), got %d", len(models[0].Files))
		}
		if models[0].Files[0] != expectedBlobHash {
			t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, models[0].Files[0])
		}
	})

	// Test AddTags
	t.Run("AddTags", func(t *testing.T) {
		err := s.AddTags("api-model:latest", []string{"api-v1.0", "api-stable"})
		if err != nil {
			t.Fatalf("AddTags failed: %v", err)
		}

		// Verify tags were added to model
		model, err := s.Read("api-model:latest")
		if err != nil {
			t.Fatalf("GetByTag failed: %v", err)
		}
		if !containsTag(model.Tags(), "api-v1.0") || !containsTag(model.Tags(), "api-stable") {
			t.Errorf("Expected new tags, got %v", model.Tags())
		}

		// Verify tags were added to list
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("Expected 1 model, got %d", len(models))
		}
		if len(models[0].Tags) != 3 {
			t.Fatalf("Expected 3 tags, got %d", len(models[0].Tags))
		}
	})

	// Test RemoveTags
	t.Run("RemoveTags", func(t *testing.T) {
		tags, err := s.RemoveTags([]string{"api-model:api-v1.0"})
		if err != nil {
			t.Fatalf("RemoveTags failed: %v", err)
		}
		// Tags are normalized with default org (ai/) prefix
		if tags[0] != "docker.io/ai/api-model:api-v1.0" {
			t.Fatalf("Expected removed tag 'docker.io/ai/api-model:api-v1.0', got '%s'", tags[0])
		}

		// Verify tag was removed from list
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		for _, model := range models {
			if containsTag(model.Tags, "api-model:api-v1.0") {
				t.Errorf("Tag should have been removed, but still present: %v", model.Tags)
			}
			if model.Files[0] != expectedBlobHash {
				t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, model.Files[0])
			}
		}

		// Verify read by tag fails
		if _, err = s.Read("api-model:api-v1.0"); err == nil {
			t.Errorf("Expected read error after tag removal, got nil")
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		_, _, err := s.Delete("api-model:latest")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify model with that tag is gone
		_, err = s.Read("api-model:latest")
		if err == nil {
			t.Errorf("Expected error after deletion, got nil")
		}
	})

	// Test Delete Non Existent Model
	t.Run("Delete", func(t *testing.T) {
		_, _, err := s.Delete("non-existent-model:latest")
		if !errors.Is(err, store.ErrModelNotFound) {
			t.Fatalf("Expected ErrModelNotFound, got %v", err)
		}
	})

	// Test that Delete removes the blob files
	t.Run("DeleteRemovesBlobs", func(t *testing.T) {
		// Create a new model with unique content
		modelContent := []byte("unique content for blob deletion test")
		modelPath := filepath.Join(tempDir, "blob-deletion-test.gguf")
		if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
			t.Fatalf("Failed to create test model file: %v", err)
		}

		// Calculate the blob hash to find it later
		hash := sha256.Sum256(modelContent)
		blobHash := hex.EncodeToString(hash[:])

		// Add model to store with a unique tag
		mdl, err := gguf.NewModel(modelPath)
		if err != nil {
			t.Fatalf("Create model failed: %v", err)
		}

		if err := s.Write(mdl, []string{"blob-test:latest", "blob-test:other"}, nil); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Get the blob path
		blobPath := filepath.Join(storePath, "blobs", "sha256", blobHash)

		// Verify the blob exists on disk before deletion
		if _, err := os.Stat(blobPath); err != nil {
			t.Fatalf("Failed to stat blob at path '%s': %v", blobPath, err)
		}

		// Get the manifest path
		digest, err := mdl.Digest()
		if err != nil {
			t.Fatalf("Failed to get digest: %v", err)
		}

		// Verify the model manifest exists
		manifestPath := filepath.Join(storePath, "manifests", "sha256", digest.Hex)
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("Failed to stat manifest at path '%s': %v", manifestPath, err)
		}

		// Delete the model
		if _, _, err := s.Delete("blob-test:latest"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify the blob no longer exists on disk after deletion
		if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
			t.Errorf("Blob file still exists after deletion: %s", blobPath)
		}

		// Verify the manifest no longer exists on disk after deletion
		if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
			t.Errorf("Manifest file still exists after deletion: %s", blobPath)
		}
	})

	// Test that shared blobs between different models are not deleted
	t.Run("SharedBlobsPreservation", func(t *testing.T) {
		// Create a model file with content that will be shared
		sharedContent := []byte("shared content for multiple models test")
		sharedModelPath := filepath.Join(tempDir, "shared-model.gguf")
		if err := os.WriteFile(sharedModelPath, sharedContent, 0644); err != nil {
			t.Fatalf("Failed to create shared model file: %v", err)
		}

		// Calculate the blob hash to find it later
		hash := sha256.Sum256(sharedContent)
		blobHash := hex.EncodeToString(hash[:])
		expectedBlobDigest := fmt.Sprintf("sha256:%s", blobHash)

		// Create first model with the shared content
		model1, err := gguf.NewModel(sharedModelPath)
		if err != nil {
			t.Fatalf("Create first model failed: %v", err)
		}

		// Write the first model
		if err := s.Write(model1, []string{"shared-model-1:latest"}, nil); err != nil {
			t.Fatalf("Write first model failed: %v", err)
		}

		// Create second model with the same shared content
		model2, err := gguf.NewModel(sharedModelPath)
		if err != nil {
			t.Fatalf("Create second model failed: %v", err)
		}

		// Write the second model
		if err := s.Write(model2, []string{"shared-model-2:latest"}, nil); err != nil {
			t.Fatalf("Write second model failed: %v", err)
		}

		// Get the blob path
		blobPath := filepath.Join(storePath, "blobs", "sha256", blobHash)

		// Get the config blob paths (not shared)
		name1, err := model1.ConfigName()
		if err != nil {
			t.Fatalf("Failed to get config name: %v", err)
		}
		config1Path := filepath.Join(storePath, "blobs", "sha256", name1.Hex)
		name2, err := model2.ConfigName()
		if err != nil {
			t.Fatalf("Failed to get config name: %v", err)
		}
		config2Path := filepath.Join(storePath, "blobs", "sha256", name2.Hex)

		// Verify the blobs exists on disk
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Fatalf("Shared blob file doesn't exist: %s", blobPath)
		}
		if _, err := os.Stat(config1Path); os.IsNotExist(err) {
			t.Fatalf("Model 1 config blob file doesn't exist: %s", config1Path)
		}
		if _, err := os.Stat(config2Path); os.IsNotExist(err) {
			t.Fatalf("Model 2 config blob file doesn't exist: %s", config2Path)
		}

		// Delete the first model
		if _, _, err := s.Delete("shared-model-1:latest"); err != nil {
			t.Fatalf("Delete first model failed: %v", err)
		}

		// Verify the shared blob still exists on disk after deleting the first model
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Shared blob file was incorrectly removed: %s", blobPath)
		}

		// Verify the first model config blob does not exist
		if _, err := os.Stat(config1Path); !os.IsNotExist(err) {
			t.Errorf("Model 1 config blob should have been removed: %s", config1Path)
		}

		// Verify the second model config blob still exists
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Model 2 config blob file was incorrectly removed: %s", config2Path)
		}

		// Verify the second model is still in the index
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		var foundModel bool
		for _, model := range models {
			if containsTag(model.Tags, "shared-model-2:latest") {
				foundModel = true
				// Verify the blob is still associated with the model
				if len(model.Files) != 2 {
					t.Errorf("Expected 2 blobs, got %d", len(model.Files))
				}
				if model.Files[0] != expectedBlobDigest {
					t.Errorf("Expected blob %s, got %v", expectedBlobDigest, model.Files)
				}
				if model.Files[1] != name2.String() {
					t.Errorf("Expected blob %s, got %v", expectedBlobDigest, model.Files)
				}
				break
			}
		}

		if !foundModel {
			t.Errorf("Second model not found after deleting first model")
		}

		// Delete the second model
		if _, _, err := s.Delete("shared-model-2:latest"); err != nil {
			t.Fatalf("Delete second model failed: %v", err)
		}

		// Now the blob should be deleted since no models reference it
		if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
			t.Errorf("Shared blob file still exists after deleting all referencing models: %s", blobPath)
		}
	})
}

func TestWriteRollsBackOnTagFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "store-rollback-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "rollback-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mdl := newTestModel(t)

	configHash, err := mdl.ConfigName()
	if err != nil {
		t.Fatalf("ConfigName failed: %v", err)
	}
	layers, err := mdl.Layers()
	if err != nil {
		t.Fatalf("Layers failed: %v", err)
	}
	var diffIDs []string
	for _, layer := range layers {
		diffID, err := layer.DiffID()
		if err != nil {
			t.Fatalf("DiffID failed: %v", err)
		}
		diffIDs = append(diffIDs, diffID.String())
	}
	digest, err := mdl.Digest()
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}

	err = s.Write(mdl, []string{"invalid tag!"}, nil)
	if err == nil {
		t.Fatalf("expected write to fail for invalid tag")
	}

	models, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("expected no models in store after failed write, found %d", len(models))
	}

	configPath := filepath.Join(storePath, "blobs", configHash.Algorithm, configHash.Hex)
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected config blob to be cleaned up, stat error: %v", err)
	}

	for _, digestStr := range diffIDs {
		parts := strings.SplitN(digestStr, ":", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected diffID format: %q", digestStr)
		}
		layerPath := filepath.Join(storePath, "blobs", parts[0], parts[1])
		if _, err := os.Stat(layerPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected layer blob %q to be cleaned up, stat error: %v", layerPath, err)
		}
	}

	manifestPath := filepath.Join(storePath, "manifests", digest.Algorithm, digest.Hex)
	if _, err := os.Stat(manifestPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected manifest to be cleaned up, stat error: %v", err)
	}

	modelsIndexPath := filepath.Join(storePath, "models.json")
	content, err := os.ReadFile(modelsIndexPath)
	if err != nil {
		t.Fatalf("failed to read models index: %v", err)
	}
	if strings.Contains(string(content), digest.Hex) {
		t.Fatalf("models index still references failed digest %s", digest.Hex)
	}
}

func TestWriteRollsBackOnConfigFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "store-config-failure")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "config-failure-store")
	s, err := store.New(store.Options{RootPath: storePath})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mdl := newTestModel(t)
	cfgFailModel := configErrorModel{ModelArtifact: mdl}

	if err := s.Write(cfgFailModel, []string{"cfg-failure:latest"}, nil); err == nil {
		t.Fatalf("expected write to fail due to config overwrite")
	}

	assertStoreClean(t, s, storePath, mdl)
}

func TestWriteRollsBackOnLayerFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "store-layer-failure")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "layer-failure-store")
	s, err := store.New(store.Options{RootPath: storePath})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mdl := newTestModel(t)
	layers, err := mdl.Layers()
	if err != nil {
		t.Fatalf("Layers failed: %v", err)
	}
	if len(layers) == 0 {
		t.Fatalf("expected at least one layer")
	}
	newHash, err := oci.NewHash("sha256:" + strings.Repeat("c", 64))
	if err != nil {
		t.Fatalf("failed to build hash: %v", err)
	}
	failing := failingLayer{Layer: layers[0], hash: newHash}
	mdl = mutate.AppendLayers(mdl, failing)

	if err := s.Write(mdl, []string{"layer-failure:latest"}, nil); err == nil {
		t.Fatalf("expected write to fail due to layer overwrite")
	}

	assertStoreClean(t, s, storePath, mdl)
}

func assertStoreClean(t *testing.T, s *store.LocalStore, storePath string, mdl types.ModelArtifact) {
	t.Helper()

	if models, err := s.List(); err != nil {
		t.Fatalf("List failed: %v", err)
	} else if len(models) != 0 {
		t.Fatalf("expected no models in store after failed write, found %d", len(models))
	}

	manifestDigest, err := mdl.Digest()
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	manifestPath := filepath.Join(storePath, "manifests", manifestDigest.Algorithm, manifestDigest.Hex)
	if _, err := os.Stat(manifestPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected manifest %s to be cleaned up, stat error: %v", manifestPath, err)
	}

	content, err := os.ReadFile(filepath.Join(storePath, "models.json"))
	if err != nil {
		t.Fatalf("failed to read models index: %v", err)
	}
	if strings.Contains(string(content), manifestDigest.Hex) {
		t.Fatalf("models index still references failed digest %s", manifestDigest.Hex)
	}
}

type configErrorModel struct {
	types.ModelArtifact
}

func (configErrorModel) RawConfigFile() ([]byte, error) {
	return nil, fmt.Errorf("forced config failure")
}

type failingLayer struct {
	oci.Layer
	hash oci.Hash
}

func (f failingLayer) DiffID() (oci.Hash, error) {
	return f.hash, nil
}

func (f failingLayer) Digest() (oci.Hash, error) {
	return f.hash, nil
}

func (f failingLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, fmt.Errorf("forced layer failure")
}

// TestIncompleteFileHandling tests that files are created with .incomplete suffix and renamed on success
func TestIncompleteFileHandling(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "incomplete-file-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary model file with known content
	modelContent := []byte("test model content for incomplete file test")
	modelPath := filepath.Join(tempDir, "incomplete-test-model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

	// Calculate expected blob hash
	hash := sha256.Sum256(modelContent)
	blobHash := hex.EncodeToString(hash[:])

	// Create store
	storePath := filepath.Join(tempDir, "incomplete-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create the blobs directory
	blobsDir := filepath.Join(storePath, "blobs", "sha256")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatalf("Failed to create blobs directory: %v", err)
	}

	// Create an incomplete file directly
	incompleteFilePath := filepath.Join(blobsDir, blobHash+".incomplete")
	if err := os.WriteFile(incompleteFilePath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create incomplete file: %v", err)
	}

	// Verify the incomplete file exists
	if _, err := os.Stat(incompleteFilePath); os.IsNotExist(err) {
		t.Fatalf("Failed to create test .incomplete file")
	}

	// Create a model
	mdl, err := gguf.NewModel(modelPath)
	if err != nil {
		t.Fatalf("Create model failed: %v", err)
	}

	// Write the model - this should clean up the incomplete file and create the final file
	if err := s.Write(mdl, []string{"incomplete-test:latest"}, nil); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify that no .incomplete files remain after successful write
	files, err := os.ReadDir(blobsDir)
	if err != nil {
		t.Fatalf("Failed to read blobs directory: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".incomplete") {
			t.Errorf("Found .incomplete file after successful write: %s", file.Name())
		}
	}

	// Verify the blob exists with its final name
	blobPath := filepath.Join(blobsDir, blobHash)
	if _, err := os.Stat(blobPath); os.IsNotExist(err) {
		t.Errorf("Blob file doesn't exist at expected path: %s", blobPath)
	}
}

// Helper function to check if a tag is in a slice of tags
func containsTag(tags []string, tag string) bool {
	// Normalize the expected tag for comparison using default registry options
	expectedRef, err := reference.ParseReference(tag, registry.GetDefaultRegistryOptions()...)
	if err != nil {
		// Fall back to exact match if parsing fails
		for _, t := range tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	expectedNorm := expectedRef.String()
	for _, t := range tags {
		// Normalize stored tag for comparison using same options
		storedRef, err := reference.ParseReference(t, registry.GetDefaultRegistryOptions()...)
		if err != nil {
			if t == tag {
				return true
			}
			continue
		}
		if storedRef.String() == expectedNorm {
			return true
		}
	}
	return false
}

func TestWriteHandlesExistingBlobsGracefully(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "store-existing-blob")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "existing-blob-store")
	s, err := store.New(store.Options{RootPath: storePath})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	model := newTestModel(t)

	if err := s.Write(model, []string{"existing:latest"}, nil); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	if err := s.Write(model, []string{"existing:latest"}, nil); err != nil {
		t.Fatalf("second write failed despite existing blobs: %v", err)
	}

	models, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected one model in store, found %d", len(models))
	}
	if !containsTag(models[0].Tags, "existing:latest") {
		t.Fatalf("expected tag existing:latest to be present, got %v", models[0].Tags)
	}
}

// TestStoreWithMultimodalProjector tests storing and retrieving models with multimodal projector files
func TestStoreWithMultimodalProjector(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "store-mmproj-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	storePath := filepath.Join(tempDir, "mmproj-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create a model with a Multimodal projector
	model := newTestModelWithMultimodalProjector(t)

	// Write the model to store
	if err := s.Write(model, []string{"mmproj-model:latest"}, nil); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read the model back
	readModel, err := s.Read("mmproj-model:latest")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify the model has MMPROJPath method
	mmprojPath, err := readModel.MMPROJPath()
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}

	if mmprojPath == "" {
		t.Error("Expected non-empty multimodal projector path")
	}

	// Verify the manifest has the correct layers
	manifest, err := readModel.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest: %v", err)
	}

	// Should have 3 layers: GGUF + license + multimodal projector
	if len(manifest.Layers) != 3 {
		t.Fatalf("Expected 3 layers, got %d", len(manifest.Layers))
	}

	// Check that one layer has the multimodal projector media type
	foundMMProjLayer := false
	for _, layer := range manifest.Layers {
		if layer.MediaType == types.MediaTypeMultimodalProjector {
			foundMMProjLayer = true
			break
		}
	}

	if !foundMMProjLayer {
		t.Error("Expected to find a layer with multimodal projector media type")
	}

	// Test List includes the multimodal projector file
	models, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	// Should have 4 files: GGUF blob, license blob, multimodal projector blob, and config
	if len(models[0].Files) != 4 {
		t.Fatalf("Expected 4 files (gguf, license, mmproj, config), got %d", len(models[0].Files))
	}
}

func newTestModel(t *testing.T) types.ModelArtifact {
	var mdl types.ModelArtifact
	var err error

	mdl, err = gguf.NewModel(filepath.Join("testdata", "dummy.gguf"))
	if err != nil {
		t.Fatalf("failed to create model from gguf file: %v", err)
	}
	licenseLayer, err := partial.NewLayer(filepath.Join("testdata", "license.txt"), types.MediaTypeLicense)
	if err != nil {
		t.Fatalf("failed to create license layer: %v", err)
	}
	mdl = mutate.AppendLayers(mdl, licenseLayer)
	return mdl
}

func newTestModelWithMultimodalProjector(t *testing.T) types.ModelArtifact {
	var mdl types.ModelArtifact
	var err error

	mdl, err = gguf.NewModel(filepath.Join("testdata", "dummy.gguf"))
	if err != nil {
		t.Fatalf("failed to create model from gguf file: %v", err)
	}

	licenseLayer, err := partial.NewLayer(filepath.Join("testdata", "license.txt"), types.MediaTypeLicense)
	if err != nil {
		t.Fatalf("failed to create license layer: %v", err)
	}

	// Create dummy multimodal projector file for testing
	tempDir, err := os.MkdirTemp("", "mmproj-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	mmprojPath := filepath.Join(tempDir, "dummy.mmproj")
	mmprojContent := []byte("dummy multimodal projector content for testing")
	if err := os.WriteFile(mmprojPath, mmprojContent, 0644); err != nil {
		t.Fatalf("failed to create dummy multimodal projector file: %v", err)
	}

	mmprojLayer, err := partial.NewLayer(mmprojPath, types.MediaTypeMultimodalProjector)
	if err != nil {
		t.Fatalf("failed to create multimodal projector layer: %v", err)
	}

	mdl = mutate.AppendLayers(mdl, licenseLayer, mmprojLayer)
	return mdl
}

// TestWriteLightweight tests the WriteLightweight method
func TestResetStore(t *testing.T) {
	tests := []struct {
		name        string
		setupModels int
	}{
		{
			name:        "reset with multiple models in store",
			setupModels: 3,
		},
		{
			name:        "reset empty store",
			setupModels: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test store
			tempDir, err := os.MkdirTemp("", "reset-store-test")
			if err != nil {
				t.Fatalf("Failed to create temp directory: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create store
			storePath := filepath.Join(tempDir, "reset-model-store")
			s, err := store.New(store.Options{
				RootPath: storePath,
			})
			if err != nil {
				t.Fatalf("Failed to create store: %v", err)
			}

			// Track blob and manifest paths for verification
			var blobPaths []string
			var manifestPaths []string

			// Setup models based on test case
			if tt.setupModels > 0 {
				for i := 0; i < tt.setupModels; i++ {
					// Create a unique model file for each iteration
					modelContent := []byte(fmt.Sprintf("unique model content %d", i))
					modelPath := filepath.Join(tempDir, fmt.Sprintf("model-%d.gguf", i))
					if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
						t.Fatalf("Failed to create model file: %v", err)
					}

					mdl, err := gguf.NewModel(modelPath)
					if err != nil {
						t.Fatalf("Failed to create model: %v", err)
					}

					tag := fmt.Sprintf("test-model-%d:latest", i)
					if err := s.Write(mdl, []string{tag}, nil); err != nil {
						t.Fatalf("Failed to write model %d: %v", i, err)
					}

					// Collect blob paths
					layers, err := mdl.Layers()
					if err != nil {
						t.Fatalf("Failed to get layers: %v", err)
					}
					for _, layer := range layers {
						digest, err := layer.Digest()
						if err != nil {
							t.Fatalf("Failed to get layer digest: %v", err)
						}
						blobPath := filepath.Join(storePath, "blobs", digest.Algorithm, digest.Hex)
						blobPaths = append(blobPaths, blobPath)
					}

					// Collect config blob path
					configName, err := mdl.ConfigName()
					if err != nil {
						t.Fatalf("Failed to get config name: %v", err)
					}
					configPath := filepath.Join(storePath, "blobs", configName.Algorithm, configName.Hex)
					blobPaths = append(blobPaths, configPath)

					// Collect manifest path
					digest, err := mdl.Digest()
					if err != nil {
						t.Fatalf("Failed to get digest: %v", err)
					}
					manifestPath := filepath.Join(storePath, "manifests", digest.Algorithm, digest.Hex)
					manifestPaths = append(manifestPaths, manifestPath)
				}

				// Verify models exist before reset
				models, err := s.List()
				if err != nil {
					t.Fatalf("Failed to list models before reset: %v", err)
				}
				if len(models) != tt.setupModels {
					t.Fatalf("Expected %d models before reset, got %d", tt.setupModels, len(models))
				}

				// Verify blobs exist before reset
				for _, blobPath := range blobPaths {
					if _, err := os.Stat(blobPath); os.IsNotExist(err) {
						t.Errorf("Blob file should exist before reset: %s", blobPath)
					}
				}

				// Verify manifests exist before reset
				for _, manifestPath := range manifestPaths {
					if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
						t.Errorf("Manifest file should exist before reset: %s", manifestPath)
					}
				}

			}

			// Call Reset
			if err := s.Reset(); err != nil {
				t.Fatalf("Reset failed: %v", err)
			}

			// Verify store is empty after reset
			models, err := s.List()
			if err != nil {
				t.Fatalf("Failed to list models after reset: %v", err)
			}
			if len(models) != 0 {
				t.Errorf("Expected empty store after reset, got %d models", len(models))
			}

			// Verify all blobs are deleted
			for _, blobPath := range blobPaths {
				if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
					t.Errorf("Blob file should be deleted after reset: %s", blobPath)
				}
			}

			// Verify all manifests are deleted
			for _, manifestPath := range manifestPaths {
				if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
					t.Errorf("Manifest file should be deleted after reset: %s", manifestPath)
				}
			}

			// Verify store root directory still exists
			if _, err := os.Stat(storePath); os.IsNotExist(err) {
				t.Error("Store directory should still exist after reset")
			}

			// Note: blobs and manifests directories are created on-demand,
			// so they won't exist after reset until models are written

			// Verify store is functional after reset by writing a new model
			newModel := newTestModel(t)
			if err := s.Write(newModel, []string{"post-reset:latest"}, nil); err != nil {
				t.Fatalf("Failed to write model after reset: %v", err)
			}

			// Verify the new model can be read
			readModel, err := s.Read("post-reset:latest")
			if err != nil {
				t.Fatalf("Failed to read model after reset: %v", err)
			}

			readDigest, err := readModel.Digest()
			if err != nil {
				t.Fatalf("Failed to get digest: %v", err)
			}

			newDigest, err := newModel.Digest()
			if err != nil {
				t.Fatalf("Failed to get new digest: %v", err)
			}

			if readDigest.String() != newDigest.String() {
				t.Error("Model written after reset doesn't match")
			}
		})
	}
}

func TestWriteLightweight(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "lightweight-write-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	storePath := filepath.Join(tempDir, "lightweight-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	t.Run("SuccessfulLightweightWrite", func(t *testing.T) {
		// Create and write a model normally to populate the store with blobs
		baseModel := newTestModel(t)
		if err := s.Write(baseModel, []string{"base-model:v1"}, nil); err != nil {
			t.Fatalf("Write base model failed: %v", err)
		}

		// Get original digest
		originalDigest, err := baseModel.Digest()
		if err != nil {
			t.Fatalf("Failed to get original digest: %v", err)
		}

		// Modify the model's config by changing context size
		newContextSize := int32(4096)
		modifiedModel := mutate.ContextSize(baseModel, newContextSize)

		// Use WriteLightweight to write the modified model
		if err := s.WriteLightweight(modifiedModel, []string{"base-model:v2"}); err != nil {
			t.Fatalf("WriteLightweight failed: %v", err)
		}

		// Verify the new model can be read
		readModel, err := s.Read("base-model:v2")
		if err != nil {
			t.Fatalf("Read modified model failed: %v", err)
		}

		// Verify the new model has a different digest
		newDigest, err := readModel.Digest()
		if err != nil {
			t.Fatalf("Failed to get new digest: %v", err)
		}

		if originalDigest.String() == newDigest.String() {
			t.Error("Expected different digest for modified model")
		}

		// Verify both models exist in the index
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(models) != 2 {
			t.Fatalf("Expected 2 models in index, got %d", len(models))
		}

		// Verify both models reference the same layer blobs
		var baseModelFiles []string
		var modifiedModelFiles []string
		for _, m := range models {
			if containsTag(m.Tags, "base-model:v1") {
				baseModelFiles = m.Files
			}
			if containsTag(m.Tags, "base-model:v2") {
				modifiedModelFiles = m.Files
			}
		}

		if len(baseModelFiles) == 0 || len(modifiedModelFiles) == 0 {
			t.Fatal("Failed to find both models in index")
		}

		// The first file should be the same (gguf blob), but config should differ
		if baseModelFiles[0] != modifiedModelFiles[0] {
			t.Error("Expected models to share the same layer blobs")
		}

		// The license blob should also be shared
		if len(baseModelFiles) >= 2 && len(modifiedModelFiles) >= 2 {
			if baseModelFiles[1] != modifiedModelFiles[1] {
				t.Error("Expected models to share the same license blob")
			}
		}
	})

	t.Run("FailureWhenLayerMissing", func(t *testing.T) {
		// Create a separate store to ensure no blobs from previous tests exist
		freshStorePath := filepath.Join(tempDir, "fresh-model-store")
		freshStore, err := store.New(store.Options{
			RootPath: freshStorePath,
		})
		if err != nil {
			t.Fatalf("Failed to create fresh store: %v", err)
		}

		// Create a new model without writing its blobs first
		freshModel := newTestModel(t)

		// Attempt to use WriteLightweight without the blobs existing
		err = freshStore.WriteLightweight(freshModel, []string{"missing-blobs:latest"})
		if err == nil {
			t.Fatal("Expected error when layer blobs are missing, got nil")
		}

		if !strings.Contains(err.Error(), "not found in store") {
			t.Errorf("Expected error about missing layer, got: %v", err)
		}
	})

	t.Run("MultipleTags", func(t *testing.T) {
		// Create and write a model normally
		baseModel := newTestModel(t)
		if err := s.Write(baseModel, []string{"multi-tag:base"}, nil); err != nil {
			t.Fatalf("Write base model failed: %v", err)
		}

		// Create a variant with different config
		newContextSize := int32(8192)
		variant := mutate.ContextSize(baseModel, newContextSize)

		// Use WriteLightweight with multiple tags
		if err := s.WriteLightweight(variant, []string{"multi-tag:variant1", "multi-tag:variant2"}); err != nil {
			t.Fatalf("WriteLightweight with multiple tags failed: %v", err)
		}

		// Verify both tags point to the same model
		model1, err := s.Read("multi-tag:variant1")
		if err != nil {
			t.Fatalf("Read variant1 failed: %v", err)
		}

		model2, err := s.Read("multi-tag:variant2")
		if err != nil {
			t.Fatalf("Read variant2 failed: %v", err)
		}

		digest1, err := model1.Digest()
		if err != nil {
			t.Fatalf("Failed to get digest1: %v", err)
		}

		digest2, err := model2.Digest()
		if err != nil {
			t.Fatalf("Failed to get digest2: %v", err)
		}

		if digest1.String() != digest2.String() {
			t.Error("Expected both tags to point to the same model")
		}

		// Verify they share the same blobs as the base model
		baseModelRead, err := s.Read("multi-tag:base")
		if err != nil {
			t.Fatalf("Read base model failed: %v", err)
		}

		baseLayers, err := baseModelRead.Layers()
		if err != nil {
			t.Fatalf("Failed to get base layers: %v", err)
		}

		variantLayers, err := model1.Layers()
		if err != nil {
			t.Fatalf("Failed to get variant layers: %v", err)
		}

		// Verify they have the same number of layers
		if len(baseLayers) != len(variantLayers) {
			t.Fatalf("Expected same number of layers, got base=%d variant=%d", len(baseLayers), len(variantLayers))
		}

		// Verify layer digests match (same blobs)
		for i := range baseLayers {
			baseDigest, err := baseLayers[i].Digest()
			if err != nil {
				t.Fatalf("Failed to get base layer digest: %v", err)
			}
			variantDigest, err := variantLayers[i].Digest()
			if err != nil {
				t.Fatalf("Failed to get variant layer digest: %v", err)
			}
			if baseDigest.String() != variantDigest.String() {
				t.Errorf("Layer %d digest mismatch", i)
			}
		}
	})

	t.Run("WithMultimodalProjector", func(t *testing.T) {
		// Create and write a model with multimodal projector
		baseModel := newTestModelWithMultimodalProjector(t)
		if err := s.Write(baseModel, []string{"mmproj-model:base"}, nil); err != nil {
			t.Fatalf("Write base model with mmproj failed: %v", err)
		}

		// Create a variant with different context size
		newContextSize := int32(2048)
		variant := mutate.ContextSize(baseModel, newContextSize)

		// Use WriteLightweight for the variant
		if err := s.WriteLightweight(variant, []string{"mmproj-model:variant"}); err != nil {
			t.Fatalf("WriteLightweight with mmproj failed: %v", err)
		}

		// Read the variant back
		readVariant, err := s.Read("mmproj-model:variant")
		if err != nil {
			t.Fatalf("Read variant failed: %v", err)
		}

		// Verify multimodal projector path exists
		mmprojPath, err := readVariant.MMPROJPath()
		if err != nil {
			t.Fatalf("Failed to get mmproj path: %v", err)
		}

		if mmprojPath == "" {
			t.Error("Expected non-empty multimodal projector path")
		}

		// Verify the manifest has all layer types
		manifest, err := readVariant.Manifest()
		if err != nil {
			t.Fatalf("Failed to get manifest: %v", err)
		}

		// Should have 3 layers: GGUF + license + multimodal projector
		if len(manifest.Layers) != 3 {
			t.Fatalf("Expected 3 layers, got %d", len(manifest.Layers))
		}

		// Verify multimodal projector layer exists
		foundMMProj := false
		for _, layer := range manifest.Layers {
			if layer.MediaType == types.MediaTypeMultimodalProjector {
				foundMMProj = true
				break
			}
		}

		if !foundMMProj {
			t.Error("Expected to find multimodal projector layer")
		}
	})

	t.Run("IndexIntegrity", func(t *testing.T) {
		// Create and write a base model
		baseModel := newTestModel(t)
		if err := s.Write(baseModel, []string{"integrity-test:base"}, nil); err != nil {
			t.Fatalf("Write base model failed: %v", err)
		}

		// Create multiple variants using WriteLightweight
		for i := 1; i <= 3; i++ {
			contextSize := int32(1024 * i)
			variant := mutate.ContextSize(baseModel, contextSize)
			tag := fmt.Sprintf("integrity-test:variant%d", i)
			if err := s.WriteLightweight(variant, []string{tag}); err != nil {
				t.Fatalf("WriteLightweight variant%d failed: %v", i, err)
			}
		}

		// Verify the index has all models
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		// Should have base + 3 variants + any models from previous tests
		// Tags are normalized to docker.io/library/integrity-test:...
		integrityTestCount := 0
		for _, m := range models {
			for _, tag := range m.Tags {
				if strings.Contains(tag, "integrity-test:") {
					integrityTestCount++
					break
				}
			}
		}

		if integrityTestCount != 4 {
			t.Fatalf("Expected 4 integrity-test models, got %d", integrityTestCount)
		}

		// Verify blob reference counts by checking that the GGUF blob
		// is listed in all 4 models' Files
		var ggufBlobHash string
		blobRefCount := 0

		for _, m := range models {
			hasIntegrityTag := false
			for _, tag := range m.Tags {
				if strings.Contains(tag, "integrity-test:") {
					hasIntegrityTag = true
					break
				}
			}

			if hasIntegrityTag {
				if len(m.Files) > 0 {
					if ggufBlobHash == "" {
						ggufBlobHash = m.Files[0]
					}
					if m.Files[0] == ggufBlobHash {
						blobRefCount++
					}
				}
			}
		}

		if blobRefCount != 4 {
			t.Errorf("Expected GGUF blob to be referenced by 4 models, got %d", blobRefCount)
		}

		// Verify the blob file exists only once on disk
		if ggufBlobHash != "" {
			// Remove the "sha256:" prefix if present
			hashStr := strings.TrimPrefix(ggufBlobHash, "sha256:")
			blobPath := filepath.Join(storePath, "blobs", "sha256", hashStr)

			if _, err := os.Stat(blobPath); os.IsNotExist(err) {
				t.Errorf("Shared blob file doesn't exist: %s", blobPath)
			}

			// Verify there's only one copy by checking file count in blobs directory
			blobsDir := filepath.Join(storePath, "blobs", "sha256")
			entries, err := os.ReadDir(blobsDir)
			if err != nil {
				t.Fatalf("Failed to read blobs directory: %v", err)
			}

			// Count non-config blobs (config blobs will be different for each variant)
			ggufCount := 0
			for _, entry := range entries {
				if !entry.IsDir() && entry.Name() == hashStr {
					ggufCount++
				}
			}

			if ggufCount != 1 {
				t.Errorf("Expected exactly 1 GGUF blob file, found %d", ggufCount)
			}
		}
	})
}
