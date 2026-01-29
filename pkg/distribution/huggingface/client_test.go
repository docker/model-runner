package huggingface

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientListFiles(t *testing.T) {
	// Mock HuggingFace API response
	mockFiles := []RepoFile{
		{Type: "file", Path: "model.safetensors", Size: 1000},
		{Type: "file", Path: "config.json", Size: 100},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/test-org/test-model/tree/main" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockFiles)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	files, err := client.ListFiles(t.Context(), "test-org/test-model", "main")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestClientListFilesDefaultRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the default revision is "main"
		if !strings.Contains(r.URL.Path, "/tree/main") {
			t.Errorf("Expected /tree/main in path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]RepoFile{})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	_, err := client.ListFiles(t.Context(), "test/model", "")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
}

func TestClientDownloadFile(t *testing.T) {
	expectedContent := "test file content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-org/test-model/resolve/main/test.txt" {
			w.Header().Set("Content-Length", "17")
			w.Write([]byte(expectedContent))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	reader, size, err := client.DownloadFile(t.Context(), "test-org/test-model", "main", "test.txt")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer reader.Close()

	if size != 17 {
		t.Errorf("Expected size 17, got %d", size)
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(content) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(content))
	}
}

func TestClientAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	_, err := client.ListFiles(t.Context(), "private/model", "main")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("Expected AuthError, got %T", err)
	}
}

func TestClientNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	_, err := client.ListFiles(t.Context(), "nonexistent/model", "main")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var notFoundErr *NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Errorf("Expected NotFoundError, got %T", err)
	}
}

func TestClientWithToken(t *testing.T) {
	var receivedToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]RepoFile{})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithToken("test-token"),
	)

	_, err := client.ListFiles(t.Context(), "test/model", "main")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if receivedToken != "Bearer test-token" {
		t.Errorf("Expected 'Bearer test-token', got %q", receivedToken)
	}
}

func TestClientListFilesInPath(t *testing.T) {
	// Mock HuggingFace API response for subdirectory
	mockFilesInSubdir := []RepoFile{
		{Type: "file", Path: "model-UD-Q4_K_XL-00001-of-00003.gguf", Size: 5000000},
		{Type: "file", Path: "model-UD-Q4_K_XL-00002-of-00003.gguf", Size: 5000000},
		{Type: "file", Path: "model-UD-Q4_K_XL-00003-of-00003.gguf", Size: 5000000},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/test-org/test-model/tree/main/UD-Q4_K_XL" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockFilesInSubdir)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	files, err := client.ListFilesInPath(t.Context(), "test-org/test-model", "main", "UD-Q4_K_XL")
	if err != nil {
		t.Fatalf("ListFilesInPath failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}

	// Verify the first file
	if files[0].Path != "model-UD-Q4_K_XL-00001-of-00003.gguf" {
		t.Errorf("Expected first file path 'model-UD-Q4_K_XL-00001-of-00003.gguf', got %q", files[0].Path)
	}
}

func TestClientListFilesInPathEmptyPath(t *testing.T) {
	// Verify that empty path uses root endpoint
	mockFiles := []RepoFile{
		{Type: "directory", Path: "UD-Q4_K_XL"},
		{Type: "file", Path: "README.md", Size: 100},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should use root endpoint without trailing slash
		if r.URL.Path == "/api/models/test-org/test-model/tree/main" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockFiles)
			return
		}
		t.Errorf("Unexpected path: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	files, err := client.ListFilesInPath(t.Context(), "test-org/test-model", "main", "")
	if err != nil {
		t.Fatalf("ListFilesInPath with empty path failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestClientListFilesRecursive(t *testing.T) {
	// Test that ListFiles recursively traverses directories
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/models/test-org/test-model/tree/main":
			// Root level: one file and one directory
			json.NewEncoder(w).Encode([]RepoFile{
				{Type: "file", Path: "README.md", Size: 100},
				{Type: "directory", Path: "models"},
			})
		case "/api/models/test-org/test-model/tree/main/models":
			// Subdirectory: one file and another nested directory
			json.NewEncoder(w).Encode([]RepoFile{
				{Type: "file", Path: "models/config.json", Size: 200},
				{Type: "directory", Path: "models/weights"},
			})
		case "/api/models/test-org/test-model/tree/main/models/weights":
			// Nested subdirectory: two files
			json.NewEncoder(w).Encode([]RepoFile{
				{Type: "file", Path: "models/weights/model-00001-of-00002.safetensors", Size: 5000000},
				{Type: "file", Path: "models/weights/model-00002-of-00002.safetensors", Size: 5000000},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	files, err := client.ListFiles(t.Context(), "test-org/test-model", "main")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	// Should return all 4 files from all levels
	if len(files) != 4 {
		t.Errorf("Expected 4 files, got %d", len(files))
		for _, f := range files {
			t.Logf("  - %s", f.Path)
		}
	}

	// Verify all expected files are present
	expectedPaths := map[string]bool{
		"README.md":          false,
		"models/config.json": false,
		"models/weights/model-00001-of-00002.safetensors": false,
		"models/weights/model-00002-of-00002.safetensors": false,
	}

	for _, f := range files {
		if _, ok := expectedPaths[f.Path]; ok {
			expectedPaths[f.Path] = true
		} else {
			t.Errorf("Unexpected file: %s", f.Path)
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("Expected file not found: %s", path)
		}
	}
}
