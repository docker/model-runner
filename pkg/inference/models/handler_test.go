package models

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/registry"

	"github.com/docker/model-runner/pkg/distribution/builder"
	reg "github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/memory"

	"github.com/sirupsen/logrus"
)

type mockMemoryEstimator struct{}

func (me *mockMemoryEstimator) SetDefaultBackend(_ memory.MemoryEstimatorBackend) {}

func (me *mockMemoryEstimator) GetRequiredMemoryForModel(_ context.Context, _ string, _ *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	return inference.RequiredMemory{RAM: 0, VRAM: 0}, nil
}

func (me *mockMemoryEstimator) HaveSufficientMemoryForModel(_ context.Context, _ string, _ *inference.BackendConfiguration) (bool, inference.RequiredMemory, inference.RequiredMemory, error) {
	return true, inference.RequiredMemory{}, inference.RequiredMemory{}, nil
}

// getProjectRoot returns the absolute path to the project root directory
func getProjectRoot(t *testing.T) string {
	// Start from the current test file's directory
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Walk up the directory tree until we find the go.mod file
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (go.mod)")
		}
		dir = parent
	}
}

func TestPullModel(t *testing.T) {

	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test registry
	server := httptest.NewServer(registry.New())
	defer server.Close()

	// Create a tag for the model
	uri, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}
	tag := uri.Host + "/ai/model:v1.0.0"

	// Prepare the OCI model artifact
	projectRoot := getProjectRoot(t)
	model, err := builder.FromGGUF(filepath.Join(projectRoot, "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model builder: %v", err)
	}

	license, err := model.WithLicense(filepath.Join(projectRoot, "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license to model: %v", err)
	}

	// Build the OCI model artifact + push it
	client := reg.NewClient()
	target, err := client.NewTarget(tag)
	if err != nil {
		t.Fatalf("Failed to create model target: %v", err)
	}
	err = license.Build(context.Background(), target, os.Stdout)
	if err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}

	tests := []struct {
		name         string
		acceptHeader string
		expectedCT   string
	}{
		{
			name:         "default content type",
			acceptHeader: "",
			expectedCT:   "text/plain",
		},
		{
			name:         "plain text content type",
			acceptHeader: "text/plain",
			expectedCT:   "text/plain",
		},
		{
			name:         "json content type",
			acceptHeader: "application/json",
			expectedCT:   "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logrus.NewEntry(logrus.StandardLogger())
			memEstimator := &mockMemoryEstimator{}
			handler := NewHandler(log, ClientConfig{
				StoreRootPath: tempDir,
				Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
			}, nil, memEstimator)

			r := httptest.NewRequest(http.MethodPost, "/models/create", strings.NewReader(`{"from": "`+tag+`"}`))
			if tt.acceptHeader != "" {
				r.Header.Set("Accept", tt.acceptHeader)
			}

			w := httptest.NewRecorder()
			err = handler.manager.Pull(tag, "", r, w)
			if err != nil {
				t.Fatalf("Failed to pull model: %v", err)
			}

			if tt.expectedCT != w.Header().Get("Content-Type") {
				t.Fatalf("Expected content type %s, got %s", tt.expectedCT, w.Header().Get("Content-Type"))
			}

			// Clean tempDir after each test
			if err := os.RemoveAll(tempDir); err != nil {
				t.Fatalf("Failed to clean temp directory: %v", err)
			}
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				t.Fatalf("Failed to recreate temp directory: %v", err)
			}
		})
	}
}

func TestHandleGetModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test registry
	server := httptest.NewServer(registry.New())
	defer server.Close()

	uri, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse registry URL: %v", err)
	}

	// Prepare the OCI model artifact
	projectRoot := getProjectRoot(t)
	model, err := builder.FromGGUF(filepath.Join(projectRoot, "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model builder: %v", err)
	}

	license, err := model.WithLicense(filepath.Join(projectRoot, "assets", "license.txt"))
	if err != nil {
		t.Fatalf("Failed to add license to model: %v", err)
	}

	// Build the OCI model artifact + push it
	tag := uri.Host + "/ai/model:v1.0.0"
	client := reg.NewClient()
	target, err := client.NewTarget(tag)
	if err != nil {
		t.Fatalf("Failed to create model target: %v", err)
	}
	err = license.Build(context.Background(), target, os.Stdout)
	if err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}

	tests := []struct {
		name          string
		remote        bool
		modelName     string
		expectedCode  int
		expectedError string
	}{
		{
			name:         "get local model - success",
			remote:       false,
			modelName:    tag,
			expectedCode: http.StatusOK,
		},
		{
			name:          "get local model - not found",
			remote:        false,
			modelName:     "nonexistent:v1",
			expectedCode:  http.StatusNotFound,
			expectedError: "model not found",
		},
		{
			name:         "get remote model - success",
			remote:       true,
			modelName:    tag,
			expectedCode: http.StatusOK,
		},
		{
			name:          "get remote model - not found",
			remote:        true,
			modelName:     uri.Host + "/ai/nonexistent:v1",
			expectedCode:  http.StatusNotFound,
			expectedError: "failed to pull model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logrus.NewEntry(logrus.StandardLogger())
			memEstimator := &mockMemoryEstimator{}
			handler := NewHandler(log, ClientConfig{
				StoreRootPath: tempDir,
				Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
				Transport:     http.DefaultTransport,
				UserAgent:     "test-agent",
			}, nil, memEstimator)

			// First pull the model if we're testing local access
			if !tt.remote && !strings.Contains(tt.modelName, "nonexistent") {
				r := httptest.NewRequest(http.MethodPost, "/models/create", strings.NewReader(`{"from": "`+tt.modelName+`"}`))
				w := httptest.NewRecorder()
				err = handler.manager.Pull(tt.modelName, "", r, w)
				if err != nil {
					t.Fatalf("Failed to pull model: %v", err)
				}
			}

			// Create request with remote query param
			path := inference.ModelsPrefix + "/" + tt.modelName
			if tt.remote {
				path += "?remote=true"
			}
			r := httptest.NewRequest(http.MethodGet, path, http.NoBody)
			w := httptest.NewRecorder()

			// Set the path value for {name} so r.PathValue("name") works
			r.SetPathValue("name", tt.modelName)

			// Call the handler directly
			handler.handleGetModel(w, r)

			// Check response
			if w.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedError != "" {
				if !strings.Contains(w.Body.String(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, w.Body.String())
				}
			} else {
				// For successful responses, verify we got a valid JSON response
				var response Model
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response body: %v", err)
				}
			}

			// Clean tempDir after each test
			if err := os.RemoveAll(tempDir); err != nil {
				t.Fatalf("Failed to clean temp directory: %v", err)
			}
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				t.Fatalf("Failed to recreate temp directory: %v", err)
			}
		})
	}
}

func TestCors(t *testing.T) {
	// Verify that preflight requests work against non-existing handlers or
	// method-specific handlers that do not support OPTIONS
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{
			name: "root",
			path: "/",
		},
		{
			name: "list models",
			path: "/models/list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			memEstimator := &mockMemoryEstimator{}
			discard := logrus.New()
			discard.SetOutput(io.Discard)
			log := logrus.NewEntry(discard)
			m := NewHandler(log, ClientConfig{}, []string{"*"}, memEstimator)
			req := httptest.NewRequest(http.MethodOptions, "http://model-runner.docker.internal"+tt.path, http.NoBody)
			req.Header.Set("Origin", "docker.com")
			w := httptest.NewRecorder()
			m.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Errorf("Expected status code 204 for OPTIONS request, got %d", w.Code)
			}
		})
	}
}
