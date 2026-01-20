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
