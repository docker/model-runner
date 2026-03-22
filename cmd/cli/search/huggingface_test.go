package search

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHuggingFaceSearchUsesVerifiedBackend(t *testing.T) {
	t.Parallel()

	server := newHuggingFaceSearchServer(t, []huggingFaceModel{
		{
			ModelID:     "stabilityai/stable-diffusion-xl-base-1.0",
			Likes:       42,
			Downloads:   1000,
			Tags:        []string{"text-to-image"},
			PipelineTag: "text-to-image",
			CreatedAt:   "2026-01-26T11:32:37.220001Z",
		},
	})
	defer server.Close()

	client := &HuggingFaceClient{
		httpClient:         server.Client(),
		baseURL:            server.URL + "/api",
		backendResolver:    fakeBackendResolver{backends: map[string]string{"stabilityai/stable-diffusion-xl-base-1.0": backendDiffusers}},
		resolveConcurrency: 1,
	}

	results, err := client.Search(t.Context(), SearchOptions{Query: "stable-diffusion", Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Backend != backendDiffusers {
		t.Fatalf("Backend = %q, want %q", results[0].Backend, backendDiffusers)
	}
}

func TestHuggingFaceSearchUsesUnknownWhenVerificationFails(t *testing.T) {
	t.Parallel()

	server := newHuggingFaceSearchServer(t, []huggingFaceModel{
		{
			ModelID:     "foo/bar",
			Likes:       1,
			Downloads:   2,
			Tags:        []string{"transformers"},
			PipelineTag: "text-generation",
			CreatedAt:   "2026-01-26T11:32:37.220001Z",
		},
	})
	defer server.Close()

	client := &HuggingFaceClient{
		httpClient:         server.Client(),
		baseURL:            server.URL + "/api",
		backendResolver:    fakeBackendResolver{errs: map[string]error{"foo/bar": errors.New("lookup failed")}},
		resolveConcurrency: 1,
	}

	results, err := client.Search(t.Context(), SearchOptions{Query: "foo", Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Backend != backendUnknown {
		t.Fatalf("Backend = %q, want %q", results[0].Backend, backendUnknown)
	}
}

func newHuggingFaceSearchServer(t *testing.T, response []huggingFaceModel) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	return httptest.NewServer(mux)
}
