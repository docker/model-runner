package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeBackendResolver struct {
	backends map[string]string
	errs     map[string]error
}

func (f fakeBackendResolver) Resolve(_ context.Context, target string) (string, int64, error) {
	if err, ok := f.errs[target]; ok {
		return backendUnknown, 0, err
	}
	if backend, ok := f.backends[target]; ok {
		return backend, 0, nil
	}
	return backendUnknown, 0, nil
}

func TestDockerHubSearchUsesVerifiedBackend(t *testing.T) {
	t.Parallel()

	server := newDockerHubSearchServer(t, dockerHubRepoListResponse{
		Results: []dockerHubRepo{
			{
				Name:        "smollm2",
				Namespace:   "ai",
				Description: "Small language model for text generation.",
				StarCount:   3,
				PullCount:   18900,
				LastUpdated: "2026-01-26T11:32:37.220001Z",
			},
		},
	})
	defer server.Close()

	client := &DockerHubClient{
		httpClient:         server.Client(),
		baseURL:            server.URL,
		backendResolver:    fakeBackendResolver{backends: map[string]string{"ai/smollm2": backendLlamaCpp}},
		resolveConcurrency: 1,
	}

	results, err := client.Search(t.Context(), SearchOptions{Query: "smollm2", Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Backend != backendLlamaCpp {
		t.Fatalf("Backend = %q, want %q", results[0].Backend, backendLlamaCpp)
	}
}

func TestDockerHubSearchUsesUnknownWhenVerificationFails(t *testing.T) {
	t.Parallel()

	server := newDockerHubSearchServer(t, dockerHubRepoListResponse{
		Results: []dockerHubRepo{
			{
				Name:        "smollm2",
				Namespace:   "ai",
				Description: "Small language model",
				StarCount:   3,
				PullCount:   18900,
				LastUpdated: "2026-01-26T11:32:37.220001Z",
			},
		},
	})
	defer server.Close()

	client := &DockerHubClient{
		httpClient:         server.Client(),
		baseURL:            server.URL,
		backendResolver:    fakeBackendResolver{errs: map[string]error{"ai/smollm2": errors.New("lookup failed")}},
		resolveConcurrency: 1,
	}

	results, err := client.Search(t.Context(), SearchOptions{Query: "smollm2", Limit: 10})
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

func newDockerHubSearchServer(t *testing.T, response dockerHubRepoListResponse) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/repositories/ai/", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	return httptest.NewServer(mux)
}
