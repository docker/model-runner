package distribution

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/name"
	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/registry"
	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/remote"
	"github.com/sirupsen/logrus"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/internal/safetensors"
	mdregistry "github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/inference/platform"
)

var (
	testGGUFFile = filepath.Join("..", "assets", "dummy.gguf")
)

func TestClientPullModel(t *testing.T) {
	// Set up test registry
	server := httptest.NewServer(registry.New())
	defer server.Close()
	registryURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}
	registry := registryURL.Host

	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Read model content for verification later
	modelContent, err := os.ReadFile(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	model, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	tag := registry + "/testmodel:v1.0.0"
	ref, err := name.ParseReference(tag)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}
	if err := remote.Write(ref, model); err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	t.Run("pull without progress writer", func(t *testing.T) {
		// Pull model from registry without progress writer
		err := client.PullModel(context.Background(), tag, nil)
		if err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		model, err := client.GetModel(tag)
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
		// Verify model content
		pulledContent, err := os.ReadFile(modelPaths[0])
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	t.Run("pull with progress writer", func(t *testing.T) {
		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model from registry with progress writer
		if err := client.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify progress output
		progressOutput := progressBuffer.String()
		if !strings.Contains(progressOutput, "Using cached model") && !strings.Contains(progressOutput, "Downloading") {
			t.Errorf("Progress output doesn't contain expected text: got %q", progressOutput)
		}

		model, err := client.GetModel(tag)
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

		// Verify model content
		pulledContent, err := os.ReadFile(modelPaths[0])
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	t.Run("pull non-existent model", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		testClient, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Test with non-existent repository
		nonExistentRef := registry + "/nonexistent/model:v1.0.0"
		err = testClient.PullModel(context.Background(), nonExistentRef, &progressBuffer)
		if err == nil {
			t.Fatal("Expected error for non-existent model, got nil")
		}

		// Verify it's a registry.Error
		var pullErr *mdregistry.Error
		ok := errors.As(err, &pullErr)
		if !ok {
			t.Fatalf("Expected PullError, got %T", err)
		}

		// Verify error fields
		if pullErr.Reference != nonExistentRef {
			t.Errorf("Expected reference %q, got %q", nonExistentRef, pullErr.Reference)
		}
		if pullErr.Code != "NAME_UNKNOWN" {
			t.Errorf("Expected error code MANIFEST_UNKNOWN, got %q", pullErr.Code)
		}
		if pullErr.Message != "Repository not found" {
			t.Errorf("Expected message '\"Repository not found', got %q", pullErr.Message)
		}
		if pullErr.Err == nil {
			t.Error("Expected underlying error to be non-nil")
		}
		if !errors.Is(pullErr, mdregistry.ErrModelNotFound) {
			t.Errorf("Expected underlying error to match ErrModelNotFound, got %v", pullErr.Err)
		}
	})

	t.Run("pull with incomplete files", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-incomplete-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		testClient, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Use the dummy.gguf file from assets directory
		mdl, err := gguf.NewModel(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		// Push model to local store
		testTag := registry + "/incomplete-test/model:v1.0.0"
		if err := testClient.store.Write(mdl, []string{testTag}, nil); err != nil {
			t.Fatalf("Failed to push model to store: %v", err)
		}

		// Push model to registry
		if err := testClient.PushModel(context.Background(), testTag, nil); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Get the model to find the GGUF path
		model, err := testClient.GetModel(testTag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		ggufPaths, err := model.GGUFPaths()
		if err != nil {
			t.Fatalf("Failed to get GGUF path: %v", err)
		}
		if len(ggufPaths) != 1 {
			t.Fatalf("Unexpected number of model files: %d", len(ggufPaths))
		}

		// Create an incomplete file by copying the GGUF file and adding .incomplete suffix
		ggufPath := ggufPaths[0]
		incompletePath := ggufPath + ".incomplete"
		originalContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read GGUF file: %v", err)
		}

		// Write partial content to simulate an incomplete download
		partialContent := originalContent[:len(originalContent)/2]
		if err := os.WriteFile(incompletePath, partialContent, 0644); err != nil {
			t.Fatalf("Failed to create incomplete file: %v", err)
		}

		// Verify the incomplete file exists
		if _, err := os.Stat(incompletePath); os.IsNotExist(err) {
			t.Fatalf("Failed to create incomplete file: %v", err)
		}

		// Delete the local model to force a pull
		if _, err := testClient.DeleteModel(testTag, false); err != nil {
			t.Fatalf("Failed to delete model: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull the model again - this should detect the incomplete file and pull again
		if err := testClient.PullModel(context.Background(), testTag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify progress output indicates a new download, not using cached model
		progressOutput := progressBuffer.String()
		if strings.Contains(progressOutput, "Using cached model") {
			t.Errorf("Expected to pull model again due to incomplete file, but used cached model")
		}

		// Verify the incomplete file no longer exists
		if _, err := os.Stat(incompletePath); !os.IsNotExist(err) {
			t.Errorf("Incomplete file still exists after successful pull: %s", incompletePath)
		}

		// Verify the complete file exists
		if _, err := os.Stat(ggufPath); os.IsNotExist(err) {
			t.Errorf("GGUF file doesn't exist after pull: %s", ggufPath)
		}

		// Verify the content of the pulled file matches the original
		pulledContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read pulled GGUF file: %v", err)
		}

		if !bytes.Equal(pulledContent, originalContent) {
			t.Errorf("Pulled content doesn't match original content")
		}
	})

	t.Run("pull updated model with same tag", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-update-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		testClient, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Read model content for verification later
		testModelContent, err := os.ReadFile(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to read test model file: %v", err)
		}

		// Push first version of model to registry
		testTag := registry + "/update-test:v1.0.0"
		if err := writeToRegistry(testGGUFFile, testTag); err != nil {
			t.Fatalf("Failed to push first version of model: %v", err)
		}

		// Pull first version of model
		if err := testClient.PullModel(context.Background(), testTag, nil); err != nil {
			t.Fatalf("Failed to pull first version of model: %v", err)
		}

		// Verify first version is in local store
		model, err := testClient.GetModel(testTag)
		if err != nil {
			t.Fatalf("Failed to get first version of model: %v", err)
		}

		modelPath, err := model.GGUFPaths()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}
		if len(modelPath) != 1 {
			t.Fatalf("Unexpected number of model files: %d", len(modelPath))
		}

		// Verify first version content
		pulledContent, err := os.ReadFile(modelPath[0])
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(testModelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, testModelContent)
		}

		// Create a modified version of the model
		updatedModelFile := filepath.Join(tempDir, "updated-dummy.gguf")
		updatedContent := append(testModelContent, []byte("UPDATED CONTENT")...)
		if err := os.WriteFile(updatedModelFile, updatedContent, 0644); err != nil {
			t.Fatalf("Failed to create updated model file: %v", err)
		}

		// Push updated model with same tag
		if err := writeToRegistry(updatedModelFile, testTag); err != nil {
			t.Fatalf("Failed to push updated model: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model again - should get the updated version
		if err := testClient.PullModel(context.Background(), testTag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull updated model: %v", err)
		}

		// Verify progress output indicates a new download, not using cached model
		progressOutput := progressBuffer.String()
		if strings.Contains(progressOutput, "Using cached model") {
			t.Errorf("Expected to pull updated model, but used cached model")
		}

		// Get the model again to verify it's the updated version
		updatedModel, err := testClient.GetModel(testTag)
		if err != nil {
			t.Fatalf("Failed to get updated model: %v", err)
		}

		updatedModelPaths, err := updatedModel.GGUFPaths()
		if err != nil {
			t.Fatalf("Failed to get updated model path: %v", err)
		}
		if len(updatedModelPaths) != 1 {
			t.Fatalf("Unexpected number of model files: %d", len(modelPath))
		}

		// Verify updated content
		updatedPulledContent, err := os.ReadFile(updatedModelPaths[0])
		if err != nil {
			t.Fatalf("Failed to read updated pulled model: %v", err)
		}

		if string(updatedPulledContent) != string(updatedContent) {
			t.Errorf("Updated pulled model content doesn't match: got %q, want %q", updatedPulledContent, updatedContent)
		}
	})

	t.Run("pull unsupported (newer) version", func(t *testing.T) {
		newMdl := mutate.ConfigMediaType(model, "application/vnd.docker.ai.model.config.v0.2+json")
		// Push model to local store
		testTag := registry + "/unsupported-test/model:v1.0.0"
		ref, err := name.ParseReference(testTag)
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}
		if err := remote.Write(ref, newMdl); err != nil {
			t.Fatalf("Failed to push model: %v", err)
		}
		if err := client.PullModel(context.Background(), testTag, nil); err == nil || !errors.Is(err, ErrUnsupportedMediaType) {
			t.Fatalf("Expected artifact version error, got %v", err)
		}
	})

	t.Run("pull safetensors model returns error on unsupported platforms", func(t *testing.T) {
		// Create temp directory for the safetensors file
		tempDir, err := os.MkdirTemp("", "safetensors-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create a minimal safetensors file (just needs to exist for this test)
		safetensorsPath := filepath.Join(tempDir, "model.safetensors")
		safetensorsContent := []byte("fake safetensors content for testing")
		if err := os.WriteFile(safetensorsPath, safetensorsContent, 0644); err != nil {
			t.Fatalf("Failed to create safetensors file: %v", err)
		}

		// Create a safetensors model
		safetensorsModel, err := safetensors.NewModel([]string{safetensorsPath})
		if err != nil {
			t.Fatalf("Failed to create safetensors model: %v", err)
		}

		// Push to registry
		testTag := registry + "/safetensors-test/model:v1.0.0"
		ref, err := name.ParseReference(testTag)
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}
		if err := remote.Write(ref, safetensorsModel); err != nil {
			t.Fatalf("Failed to push safetensors model to registry: %v", err)
		}

		// Create a new client with a separate temp store
		clientTempDir, err := os.MkdirTemp("", "client-safetensors-test-*")
		if err != nil {
			t.Fatalf("Failed to create client temp directory: %v", err)
		}
		defer os.RemoveAll(clientTempDir)

		testClient, err := NewClient(WithStoreRootPath(clientTempDir))
		if err != nil {
			t.Fatalf("Failed to create test client: %v", err)
		}

		// Try to pull the safetensors model with a progress writer to capture warnings
		var progressBuf bytes.Buffer
		err = testClient.PullModel(context.Background(), testTag, &progressBuf)

		// Pull should succeed on all platforms now (with a warning on non-Linux)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !platform.SupportsVLLM() {
			// On non-Linux, verify that a warning was written
			progressOutput := progressBuf.String()
			if !strings.Contains(progressOutput, `"type":"warning"`) {
				t.Fatalf("Expected warning message on non-Linux platforms, got output: %s", progressOutput)
			}
			if !strings.Contains(progressOutput, warnUnsupportedFormat) {
				t.Fatalf("Expected warning about safetensors format, got output: %s", progressOutput)
			}
		}
	})

	t.Run("pull with JSON progress messages", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-json-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		testClient, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model from registry with progress writer
		if err := testClient.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Parse progress output as JSON
		var messages []progress.Message
		scanner := bufio.NewScanner(&progressBuffer)
		for scanner.Scan() {
			line := scanner.Text()
			var msg progress.Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				t.Fatalf("Failed to parse JSON progress message: %v, line: %s", err, line)
			}
			messages = append(messages, msg)
		}

		if err := scanner.Err(); err != nil {
			t.Fatalf("Error reading progress output: %v", err)
		}

		// Verify we got some messages
		if len(messages) == 0 {
			t.Fatal("No progress messages received")
		}

		// Check the last message is a success message
		lastMsg := messages[len(messages)-1]
		if lastMsg.Type != "success" {
			t.Errorf("Expected last message to be success, got type: %s, message: %s", lastMsg.Type, lastMsg.Message)
		}

		// Verify model was pulled correctly
		model, err := testClient.GetModel(tag)
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

		// Verify model content
		pulledContent, err := os.ReadFile(modelPaths[0])
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original")
		}
	})

	t.Run("pull with error and JSON progress messages", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-json-error-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		testClient, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Test with non-existent model
		nonExistentRef := registry + "/nonexistent/model:v1.0.0"
		err = testClient.PullModel(context.Background(), nonExistentRef, &progressBuffer)

		// Expect an error
		if err == nil {
			t.Fatal("Expected error for non-existent model, got nil")
		}

		// Verify it matches registry.ErrModelNotFound
		if !errors.Is(err, mdregistry.ErrModelNotFound) {
			t.Fatalf("Expected registry.ErrModelNotFound, got %T", err)
		}

		// No JSON messages should be in the buffer for this error case
		// since the error happens before we start streaming progress
	})
}

func TestClientGetModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create model from test GGUF file
	model, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Write(model, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Get model
	mi, err := client.GetModel(tag)
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	// Verify model
	if len(mi.Tags()) == 0 || mi.Tags()[0] != tag {
		t.Errorf("Model tags don't match: got %v, want [%s]", mi.Tags(), tag)
	}
}

func TestClientGetModelNotFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get non-existent model
	_, err = client.GetModel("nonexistent/model:v1.0.0")
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("Expected ErrModelNotFound, got %v", err)
	}
}

func TestClientListModels(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create test model file
	modelContent := []byte("test model content")
	modelFile := filepath.Join(tempDir, "test-model.gguf")
	if err := os.WriteFile(modelFile, modelContent, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}

	mdl, err := gguf.NewModel(modelFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Push models to local store with different manifest digests
	// First model
	tag1 := "test/model1:v1.0.0"
	if err := client.store.Write(mdl, []string{tag1}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Create a slightly different model file for the second model
	modelContent2 := []byte("test model content 2")
	modelFile2 := filepath.Join(tempDir, "test-model2.gguf")
	if err := os.WriteFile(modelFile2, modelContent2, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}
	mdl2, err := gguf.NewModel(modelFile2)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Second model
	tag2 := "test/model2:v1.0.0"
	if err := client.store.Write(mdl2, []string{tag2}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Tags for verification
	tags := []string{tag1, tag2}

	// List models
	models, err := client.ListModels()
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}

	// Verify models
	if len(models) != len(tags) {
		t.Errorf("Expected %d models, got %d", len(tags), len(models))
	}

	// Check if all tags are present
	tagMap := make(map[string]bool)
	for _, model := range models {
		for _, tag := range model.Tags() {
			tagMap[tag] = true
		}
	}

	for _, tag := range tags {
		if !tagMap[tag] {
			t.Errorf("Tag %s not found in models", tag)
		}
	}
}

func TestClientGetStorePath(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get store path
	storePath := client.GetStorePath()

	// Verify store path matches the temp directory
	if storePath != tempDir {
		t.Errorf("Store path doesn't match: got %s, want %s", storePath, tempDir)
	}

	// Verify the store directory exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Errorf("Store directory does not exist: %s", storePath)
	}
}

func TestClientDefaultLogger(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client without specifying logger
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify that logger is not nil
	if client.log == nil {
		t.Error("Default logger should not be nil")
	}

	// Create client with custom logger
	customLogger := logrus.NewEntry(logrus.New())
	client, err = NewClient(
		WithStoreRootPath(tempDir),
		WithLogger(customLogger),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify that custom logger is used
	if client.log != customLogger {
		t.Error("Custom logger should be used when specified")
	}
}

func TestWithFunctionsNilChecks(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test WithStoreRootPath with empty string
	t.Run("WithStoreRootPath empty string", func(t *testing.T) {
		// Create options with a valid path first
		opts := defaultOptions()
		WithStoreRootPath(tempDir)(opts)

		// Then try to override with empty string
		WithStoreRootPath("")(opts)

		// Verify the path wasn't changed to empty
		if opts.storeRootPath != tempDir {
			t.Errorf("WithStoreRootPath with empty string changed the path: got %q, want %q",
				opts.storeRootPath, tempDir)
		}
	})

	// Test WithLogger with nil
	t.Run("WithLogger nil", func(t *testing.T) {
		// Create options with default logger
		opts := defaultOptions()
		defaultLogger := opts.logger

		// Try to override with nil
		WithLogger(nil)(opts)

		// Verify the logger wasn't changed to nil
		if opts.logger == nil {
			t.Error("WithLogger with nil changed logger to nil")
		}

		// Verify it's still the default logger
		if opts.logger != defaultLogger {
			t.Error("WithLogger with nil changed the logger")
		}
	})

	// Test WithTransport with nil
	t.Run("WithTransport nil", func(t *testing.T) {
		// Create options with default transport
		opts := defaultOptions()
		defaultTransport := opts.transport

		// Try to override with nil
		WithTransport(nil)(opts)

		// Verify the transport wasn't changed to nil
		if opts.transport == nil {
			t.Error("WithTransport with nil changed transport to nil")
		}

		// Verify it's still the default transport
		if opts.transport != defaultTransport {
			t.Error("WithTransport with nil changed the transport")
		}
	})

	// Test WithUserAgent with empty string
	t.Run("WithUserAgent empty string", func(t *testing.T) {
		// Create options with default user agent
		opts := defaultOptions()
		defaultUA := opts.userAgent

		// Try to override with empty string
		WithUserAgent("")(opts)

		// Verify the user agent wasn't changed to empty
		if opts.userAgent == "" {
			t.Error("WithUserAgent with empty string changed user agent to empty")
		}

		// Verify it's still the default user agent
		if opts.userAgent != defaultUA {
			t.Errorf("WithUserAgent with empty string changed the user agent: got %q, want %q",
				opts.userAgent, defaultUA)
		}
	})
}

func TestNewReferenceError(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test with invalid reference
	invalidRef := "invalid:reference:format"
	err = client.PullModel(context.Background(), invalidRef, nil)
	if err == nil {
		t.Fatal("Expected error for invalid reference, got nil")
	}

	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("Expected error to match sentinel invalid reference error, got %v", err)
	}
}

func TestPush(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test registry
	server := httptest.NewServer(registry.New())
	defer server.Close()

	// Create a tag for the model
	uri, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}
	tag := uri.Host + "/incomplete-test/model:v1.0.0"

	// Write a test model to the store with the given tag
	mdl, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	digest, err := mdl.ID()
	if err != nil {
		t.Fatalf("Failed to get digest of original model: %v", err)
	}

	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Push the model to the registry
	if err := client.PushModel(context.Background(), tag, nil); err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	// Delete local copy (so we can test pulling)
	if _, err := client.DeleteModel(tag, false); err != nil {
		t.Fatalf("Failed to delete model: %v", err)
	}

	// Test that model can be pulled successfully
	if err := client.PullModel(context.Background(), tag, nil); err != nil {
		t.Fatalf("Failed to pull model: %v", err)
	}

	// Test that model the pulled model is the same as the original (matching digests)
	mdl2, err := client.GetModel(tag)
	if err != nil {
		t.Fatalf("Failed to get pulled model: %v", err)
	}
	digest2, err := mdl2.ID()
	if err != nil {
		t.Fatalf("Failed to get digest of the pulled model: %v", err)
	}
	if digest != digest2 {
		t.Fatalf("Digests don't match: got %s, want %s", digest2, digest)
	}
}

func TestPushProgress(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test registry
	server := httptest.NewServer(registry.New())
	defer server.Close()

	// Create a tag for the model
	uri, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}
	tag := uri.Host + "/some/model/repo:some-tag"

	// Create random "model" of a given size - make it large enough to ensure multiple updates
	// We want at least 2MB to ensure we get both time-based and byte-based updates
	sz := int64(progress.MinBytesForUpdate * 2)
	path, err := randomFile(sz)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(path)

	mdl, err := gguf.NewModel(path)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	// Create a buffer to capture progress output
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		defer pw.Close()
		done <- client.PushModel(t.Context(), tag, pw)
		close(done)
	}()

	var lines []string
	sc := bufio.NewScanner(pr)
	for sc.Scan() {
		line := sc.Text()
		t.Log(line)
		lines = append(lines, line)
	}

	// Wait for the push to complete
	if err := <-done; err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	// Verify we got at least 3 messages (2 progress + 1 success)
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 progress messages, got %d", len(lines))
	}

	// Verify the last two messages
	lastTwo := lines[len(lines)-2:]
	if !strings.Contains(lastTwo[0], "Uploaded:") {
		t.Fatalf("Expected progress message to contain 'Uploaded: x MB', got %q", lastTwo[0])
	}
	if !strings.Contains(lastTwo[1], "success") {
		t.Fatalf("Expected last progress message to contain 'success', got %q", lastTwo[1])
	}
}

func TestTag(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test model
	model, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	id, err := model.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}

	// Normalize the model name before writing
	normalized := client.normalizeModelName("some-repo:some-tag")

	// Push the model to the store
	if err := client.store.Write(model, []string{normalized}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Tag the model by ID
	if err := client.Tag(id, "other-repo:tag1"); err != nil {
		t.Fatalf("Failed to tag model %q: %v", id, err)
	}

	// Tag the model by tag
	if err := client.Tag(id, "other-repo:tag2"); err != nil {
		t.Fatalf("Failed to tag model %q: %v", id, err)
	}

	// Verify the model has all 3 tags
	modelInfo, err := client.GetModel("some-repo:some-tag")
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	if len(modelInfo.Tags()) != 3 {
		t.Fatalf("Expected 3 tags, got %d", len(modelInfo.Tags()))
	}

	// Verify the model can be accessed by new tags
	if _, err := client.GetModel("other-repo:tag1"); err != nil {
		t.Fatalf("Failed to get model by tag: %v", err)
	}
	if _, err := client.GetModel("other-repo:tag2"); err != nil {
		t.Fatalf("Failed to get model by tag: %v", err)
	}
}

func TestTagNotFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Tag the model by ID
	if err := client.Tag("non-existent-model:latest", "other-repo:tag1"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Expected ErrModelNotFound, got: %v", err)
	}
}

func TestClientPushModelNotFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if err := client.PushModel(t.Context(), "non-existent-model:latest", nil); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Expected ErrModelNotFound got: %v", err)
	}
}

func TestIsModelInStoreNotFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if inStore, err := client.IsModelInStore("non-existent-model:latest"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	} else if inStore {
		t.Fatalf("Expected model not to be found")
	}
}

func TestIsModelInStoreFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test model
	model, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Normalize the model name before writing
	normalized := client.normalizeModelName("some-repo:some-tag")

	// Push the model to the store
	if err := client.store.Write(model, []string{normalized}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	if inStore, err := client.IsModelInStore("some-repo:some-tag"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	} else if !inStore {
		t.Fatalf("Expected model to be found")
	}
}

// writeToRegistry writes a GGUF model to a registry.
func writeToRegistry(source, reference string) error {

	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("parse ref: %w", err)
	}

	// Create image with layer
	mdl, err := gguf.NewModel(source)
	if err != nil {
		return fmt.Errorf("new model: %w", err)
	}

	// Push the image
	if err := remote.Write(ref, mdl); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

func randomFile(size int64) (string, error) {
	// Create a temporary "gguf" file
	f, err := os.CreateTemp("", "test-*.gguf")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp file: %v", err))
	}
	defer f.Close()

	// Fill with random data
	if _, err := io.Copy(f, io.LimitReader(rand.Reader, size)); err != nil {
		return "", fmt.Errorf("Failed to write random data: %w", err)
	}

	return f.Name(), nil
}
