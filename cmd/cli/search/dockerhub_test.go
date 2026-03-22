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

func (f fakeBackendResolver) Resolve(_ context.Context, target string) (string, error) {
	if err, ok := f.errs[target]; ok {
		return backendUnknown, err
	}
	if backend, ok := f.backends[target]; ok {
		return backend, nil
	}
	return backendUnknown, nil
}

func TestDockerHubSearchUsesVerifiedBackend(t *testing.T) {
	t.Parallel()

	server := newDockerHubSearchServer(t, dockerHubRepoListResponse{
		Results: []dockerHubRepo{
			{
				Name:        "stable-diffusion",
				Namespace:   "ai",
				Description: "Image generation model, uses a base latent diffusion model plus a refiner.",
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
		backendResolver:    fakeBackendResolver{backends: map[string]string{"ai/stable-diffusion": backendDiffusers}},
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

func TestDockerHubSearchUsesUnknownWhenVerificationFails(t *testing.T) {
	t.Parallel()

	server := newDockerHubSearchServer(t, dockerHubRepoListResponse{
		Results: []dockerHubRepo{
			{
				Name:        "stable-diffusion",
				Namespace:   "ai",
				Description: "Image generation model",
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
		backendResolver:    fakeBackendResolver{errs: map[string]error{"ai/stable-diffusion": errors.New("lookup failed")}},
		resolveConcurrency: 1,
	}

	results, err := client.Search(t.Context(), SearchOptions{Query: "stable-diffusion", Limit: 10})
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
