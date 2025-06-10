package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/docker/model-distribution/builder"
	reg "github.com/docker/model-distribution/registry"
	"github.com/docker/model-runner/pkg/inference"

	"github.com/sirupsen/logrus"
)

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
			m := NewManager(log, ClientConfig{
				StoreRootPath: tempDir,
				Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
			}, nil)

			r := httptest.NewRequest("POST", "/models/create", strings.NewReader(`{"from": "`+tag+`"}`))
			if tt.acceptHeader != "" {
				r.Header.Set("Accept", tt.acceptHeader)
			}

			w := httptest.NewRecorder()
			err = m.PullModel(tag, r, w)
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
			expectedError: "error while getting model",
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
			expectedError: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logrus.NewEntry(logrus.StandardLogger())
			m := NewManager(log, ClientConfig{
				StoreRootPath: tempDir,
				Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
				Transport:     http.DefaultTransport,
				UserAgent:     "test-agent",
			}, nil)

			// First pull the model if we're testing local access
			if !tt.remote && !strings.Contains(tt.modelName, "nonexistent") {
				r := httptest.NewRequest("POST", "/models/create", strings.NewReader(`{"from": "`+tt.modelName+`"}`))
				w := httptest.NewRecorder()
				err = m.PullModel(tt.modelName, r, w)
				if err != nil {
					t.Fatalf("Failed to pull model: %v", err)
				}
			}

			// Create request with remote query param
			path := inference.ModelsPrefix + "/" + tt.modelName
			if tt.remote {
				path += "?remote=true"
			}
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			// Set the path value for {name} so r.PathValue("name") works
			r.SetPathValue("name", tt.modelName)

			// Call the handler directly
			m.handleGetModel(w, r)

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
