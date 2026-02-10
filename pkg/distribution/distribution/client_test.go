package distribution

import (
	"bufio"
	"bytes"
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

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/internal/safetensors"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
	mdregistry "github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/registry/testregistry"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/sirupsen/logrus"
)

var (
	testGGUFFile = filepath.Join("..", "assets", "dummy.gguf")
)

// newTestClient creates a new client configured for testing with plain HTTP enabled.
func newTestClient(storeRootPath string) (*Client, error) {
	return NewClient(
		WithStoreRootPath(storeRootPath),
		WithRegistryClient(mdregistry.NewClient(mdregistry.WithPlainHTTP(true))),
	)
}

func TestClientPullModel(t *testing.T) {
	// Set up test registry
	server := httptest.NewServer(testregistry.New())
	defer server.Close()
	registryURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}
	registryHost := registryURL.Host

	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	tag := registryHost + "/testmodel:v1.0.0"
	ref, err := reference.ParseReference(tag)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}
	if err := remote.Write(ref, model, nil, remote.WithPlainHTTP(true)); err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	t.Run("pull without progress writer", func(t *testing.T) {
		// Pull model from registry without progress writer
		err := client.PullModel(t.Context(), tag, nil)
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
		if err := client.PullModel(t.Context(), tag, &progressBuffer); err != nil {
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
		tempDir := t.TempDir()

		// Create client with plainHTTP for test registry
		testClient, err := newTestClient(tempDir)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Test with non-existent repository
		nonExistentRef := registryHost + "/nonexistent/model:v1.0.0"
		err = testClient.PullModel(t.Context(), nonExistentRef, &progressBuffer)
		if err == nil {
			t.Fatal("Expected error for non-existent model, got nil")
		}

		// Verify it's a registry.Error
		var pullErr *mdregistry.Error
		ok := errors.As(err, &pullErr)
		if !ok {
			t.Fatalf("Expected registry.Error, got %T: %v", err, err)
		}

		// Verify it matches registry.ErrModelNotFound for API compatibility
		if !errors.Is(err, mdregistry.ErrModelNotFound) {
			t.Fatalf("Expected registry.ErrModelNotFound, got %T", err)
		}

		// Verify error fields
		if pullErr.Reference != nonExistentRef {
			t.Errorf("Expected reference %q, got %q", nonExistentRef, pullErr.Reference)
		}
		// The error code can be NAME_UNKNOWN, MANIFEST_UNKNOWN, or UNKNOWN depending on the resolver implementation
		if pullErr.Code != "NAME_UNKNOWN" && pullErr.Code != "MANIFEST_UNKNOWN" && pullErr.Code != "UNKNOWN" {
			t.Errorf("Expected error code NAME_UNKNOWN, MANIFEST_UNKNOWN, or UNKNOWN, got %q", pullErr.Code)
		}
		// The error message varies by resolver implementation
		if !strings.Contains(strings.ToLower(pullErr.Message), "not found") {
			t.Errorf("Expected message to contain 'not found', got %q", pullErr.Message)
		}
		if pullErr.Err == nil {
			t.Error("Expected underlying error to be non-nil")
		}
	})

	t.Run("pull with incomplete files", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create client with plainHTTP for test registry
		testClient, err := newTestClient(tempDir)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Use the dummy.gguf file from assets directory
		mdl, err := gguf.NewModel(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		// Push model to local store
		testTag := registryHost + "/incomplete-test/model:v1.0.0"
		if err := testClient.store.Write(mdl, []string{testTag}, nil); err != nil {
			t.Fatalf("Failed to push model to store: %v", err)
		}

		// Push model to registry
		if err := testClient.PushModel(t.Context(), testTag, nil); err != nil {
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
		if err := testClient.PullModel(t.Context(), testTag, &progressBuffer); err != nil {
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
		tempDir := t.TempDir()

		// Create client with plainHTTP for test registry
		testClient, err := newTestClient(tempDir)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Read model content for verification later
		testModelContent, err := os.ReadFile(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to read test model file: %v", err)
		}

		// Push first version of model to registry
		testTag := registryHost + "/update-test:v1.0.0"
		if err := writeToRegistry(testGGUFFile, testTag, remote.WithPlainHTTP(true)); err != nil {
			t.Fatalf("Failed to push first version of model: %v", err)
		}

		// Pull first version of model
		if err := testClient.PullModel(t.Context(), testTag, nil); err != nil {
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
		if err := writeToRegistry(updatedModelFile, testTag, remote.WithPlainHTTP(true)); err != nil {
			t.Fatalf("Failed to push updated model: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model again - should get the updated version
		if err := testClient.PullModel(t.Context(), testTag, &progressBuffer); err != nil {
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
		testTag := registryHost + "/unsupported-test/model:v1.0.0"
		ref, err := reference.ParseReference(testTag)
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}
		if err := remote.Write(ref, newMdl, nil, remote.WithPlainHTTP(true)); err != nil {
			t.Fatalf("Failed to push model: %v", err)
		}
		if err := client.PullModel(t.Context(), testTag, nil); err == nil || !errors.Is(err, ErrUnsupportedMediaType) {
			t.Fatalf("Expected artifact version error, got %v", err)
		}
	})

	t.Run("pull safetensors model returns error on unsupported platforms", func(t *testing.T) {
		safetensorsTempDir := t.TempDir()

		// Create a minimal safetensors file (just needs to exist for this test)
		safetensorsPath := filepath.Join(safetensorsTempDir, "model.safetensors")
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
		testTag := registryHost + "/safetensors-test/model:v1.0.0"
		ref, err := reference.ParseReference(testTag)
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}
		if err := remote.Write(ref, safetensorsModel, nil, remote.WithPlainHTTP(true)); err != nil {
			t.Fatalf("Failed to push safetensors model to registry: %v", err)
		}

		// Create a new client with a separate temp store
		clientTempDir := t.TempDir()

		testClient, err := newTestClient(clientTempDir)
		if err != nil {
			t.Fatalf("Failed to create test client: %v", err)
		}

		// Try to pull the safetensors model with a progress writer to capture warnings
		var progressBuf bytes.Buffer
		err = testClient.PullModel(t.Context(), testTag, &progressBuf)

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
		tempDir := t.TempDir()

		// Create client with plainHTTP for test registry
		testClient, err := newTestClient(tempDir)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model from registry with progress writer
		if err := testClient.PullModel(t.Context(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Parse progress output as JSON
		var messages []oci.ProgressMessage
		scanner := bufio.NewScanner(&progressBuffer)
		for scanner.Scan() {
			line := scanner.Text()
			var msg oci.ProgressMessage
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

		// Verify all messages have the correct mode
		for i, msg := range messages {
			if msg.Mode != oci.ModePull {
				t.Errorf("message %d: expected mode %q, got %q", i, oci.ModePull, msg.Mode)
			}
		}

		// Check the last message is a success message
		lastMsg := messages[len(messages)-1]
		if lastMsg.Type != oci.TypeSuccess {
			t.Errorf("Expected last message to be success, got type: %q, message: %s", lastMsg.Type, lastMsg.Message)
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
		tempDir := t.TempDir()

		// Create client with plainHTTP for test registry
		testClient, err := newTestClient(tempDir)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Test with non-existent model
		nonExistentRef := registryHost + "/nonexistent/model:v1.0.0"
		err = testClient.PullModel(t.Context(), nonExistentRef, &progressBuffer)

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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	normalizedTag := "docker.io/test/model:v1.0.0" // Reference package normalizes to include registry
	if err := client.store.Write(model, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Get model
	mi, err := client.GetModel(tag)
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	// Verify model - tags are normalized to include the default registry
	if len(mi.Tags()) == 0 || mi.Tags()[0] != normalizedTag {
		t.Errorf("Model tags don't match: got %v, want [%s]", mi.Tags(), normalizedTag)
	}
}

func TestClientGetModelNotFound(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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

	// Normalized tags for verification (reference package normalizes to include default registry)
	normalizedTag1 := "docker.io/test/model1:v1.0.0"
	normalizedTag2 := "docker.io/test/model2:v1.0.0"
	tags := []string{normalizedTag1, normalizedTag2}

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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	tempDir := t.TempDir()

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
	tempDir := t.TempDir()

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

	t.Run("WithRegistryClient nil", func(t *testing.T) {
		opts := defaultOptions()
		opts.registryClient = mdregistry.NewClient()

		WithRegistryClient(nil)(opts)

		if opts.registryClient == nil {
			t.Error("WithRegistryClient with nil changed registryClient to nil")
		}
	})
}

func TestNewReferenceError(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test with invalid reference
	invalidRef := "invalid:reference:format"
	err = client.PullModel(t.Context(), invalidRef, nil)
	if err == nil {
		t.Fatal("Expected error for invalid reference, got nil")
	}

	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("Expected error to match sentinel invalid reference error, got %v", err)
	}
}

func TestPush(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test registry
	server := httptest.NewServer(testregistry.New())
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
	if err := client.PushModel(t.Context(), tag, nil); err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	// Delete local copy (so we can test pulling)
	if _, err := client.DeleteModel(tag, false); err != nil {
		t.Fatalf("Failed to delete model: %v", err)
	}

	// Test that model can be pulled successfully
	if err := client.PullModel(t.Context(), tag, nil); err != nil {
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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a test registry
	server := httptest.NewServer(testregistry.New())
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

	// Verify we got at least 2 messages (1 progress + 1 success)
	// With fast local uploads, we may only get one progress update per layer
	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 progress messages, got %d", len(lines))
	}

	// Verify we got at least one progress message and the success message
	hasProgress := false
	hasSuccess := false
	for _, line := range lines {
		if strings.Contains(line, "Uploaded:") {
			hasProgress = true
		}
		if strings.Contains(line, "success") {
			hasSuccess = true
		}
	}
	if !hasProgress {
		t.Fatalf("Expected at least one progress message containing 'Uploaded:', got %v", lines)
	}
	if !hasSuccess {
		t.Fatalf("Expected a success message, got %v", lines)
	}
}

func TestTag(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Tag the model by ID
	if err := client.Tag("non-existent-model:latest", "other-repo:tag1"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Expected ErrModelNotFound, got: %v", err)
	}
}

func TestClientPushModelNotFound(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if err := client.PushModel(t.Context(), "non-existent-model:latest", nil); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Expected ErrModelNotFound got: %v", err)
	}
}

func TestIsModelInStoreNotFound(t *testing.T) {
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
	tempDir := t.TempDir()

	// Create client with plainHTTP for test registry
	client, err := newTestClient(tempDir)
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
func writeToRegistry(source, refStr string, opts ...remote.Option) error {

	// Parse the reference
	ref, err := reference.ParseReference(refStr)
	if err != nil {
		return fmt.Errorf("parse ref: %w", err)
	}

	// Create image with layer
	mdl, err := gguf.NewModel(source)
	if err != nil {
		return fmt.Errorf("new model: %w", err)
	}

	// Push the image
	if err := remote.Write(ref, mdl, nil, opts...); err != nil {
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

func TestMigrateHFTagsOnClientInit(t *testing.T) {
	testCases := []struct {
		name          string
		storedTag     string
		lookupRef     string
		shouldMigrate bool
	}{
		{
			name:          "hf.co tag migrated to huggingface.co on init",
			storedTag:     "hf.co/testorg/testmodel:latest",
			lookupRef:     "hf.co/testorg/testmodel",
			shouldMigrate: true,
		},
		{
			name:          "hf.co tag with quantization migrated",
			storedTag:     "hf.co/bartowski/llama-3.2-1b-instruct-gguf:Q4_K_M",
			lookupRef:     "hf.co/bartowski/llama-3.2-1b-instruct-gguf:Q4_K_M",
			shouldMigrate: true,
		},
		{
			name:          "huggingface.co tag unchanged",
			storedTag:     "huggingface.co/testorg/testmodel:latest",
			lookupRef:     "huggingface.co/testorg/testmodel",
			shouldMigrate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Step 1: Create a client and write a model with the legacy tag
			setupClient, err := newTestClient(tempDir)
			if err != nil {
				t.Fatalf("Failed to create setup client: %v", err)
			}

			model, err := gguf.NewModel(testGGUFFile)
			if err != nil {
				t.Fatalf("Failed to create model: %v", err)
			}

			if err := setupClient.store.Write(model, []string{tc.storedTag}, nil); err != nil {
				t.Fatalf("Failed to write model to store: %v", err)
			}

			// Step 2: Create a NEW client (simulating restart) - migration should happen
			client, err := newTestClient(tempDir)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Step 3: Verify the model can be found using the reference
			// (normalizeModelName converts hf.co -> huggingface.co, and migration should have updated the store)
			foundModel, err := client.GetModel(tc.lookupRef)
			if err != nil {
				t.Fatalf("Failed to get model after migration: %v", err)
			}

			if foundModel == nil {
				t.Fatal("Expected to find model after migration, got nil")
			}

			// Step 4: If the tag was hf.co, verify it was actually migrated in the store
			if tc.shouldMigrate {
				// The model should now have huggingface.co tag, not hf.co
				tags := foundModel.Tags()
				hasOldTag := false
				hasNewTag := false
				for _, tag := range tags {
					if strings.HasPrefix(tag, "hf.co/") {
						hasOldTag = true
					}
					if strings.HasPrefix(tag, "huggingface.co/") {
						hasNewTag = true
					}
				}
				if hasOldTag {
					t.Errorf("Model still has old hf.co tag after migration: %v", tags)
				}
				if !hasNewTag {
					t.Errorf("Model doesn't have huggingface.co tag after migration: %v", tags)
				}
			}
		})
	}
}

func TestPullHuggingFaceModelFromCache(t *testing.T) {
	testCases := []struct {
		name    string
		pullRef string
	}{
		{
			name:    "full URL",
			pullRef: "huggingface.co/testorg/testmodel:latest",
		},
		{
			name:    "short URL",
			pullRef: "hf.co/testorg/testmodel:latest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create client
			client, err := newTestClient(tempDir)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Create a test model and write it to the store with a normalized HuggingFace tag
			model, err := gguf.NewModel(testGGUFFile)
			if err != nil {
				t.Fatalf("Failed to create model: %v", err)
			}

			// Store with normalized tag (huggingface.co)
			hfTag := "huggingface.co/testorg/testmodel:latest"
			if err := client.store.Write(model, []string{hfTag}, nil); err != nil {
				t.Fatalf("Failed to write model to store: %v", err)
			}

			// Now try to pull using the test case's reference - it should use the cache
			var progressBuffer bytes.Buffer
			err = client.PullModel(t.Context(), tc.pullRef, &progressBuffer)
			if err != nil {
				t.Fatalf("Failed to pull model from cache: %v", err)
			}

			// Verify that progress shows it was cached
			progressOutput := progressBuffer.String()
			if !strings.Contains(progressOutput, "Using cached model") {
				t.Errorf("Expected progress to indicate cached model, got: %s", progressOutput)
			}
		})
	}
}
