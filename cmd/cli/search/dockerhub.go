package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	dockerHubBaseURL = "https://hub.docker.com"
	dockerHubAIOrg   = "ai"
)

// DockerHubClient searches for models on Docker Hub
type DockerHubClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewDockerHubClient creates a new Docker Hub search client
func NewDockerHubClient() *DockerHubClient {
	return &DockerHubClient{
		httpClient: NewHTTPClient(),
		baseURL:    dockerHubBaseURL,
	}
}

// dockerHubRepoListResponse is the response from Docker Hub's repository list API
type dockerHubRepoListResponse struct {
	Count    int             `json:"count"`
	Next     string          `json:"next"`
	Previous string          `json:"previous"`
	Results  []dockerHubRepo `json:"results"`
}

// dockerHubRepo represents a repository on Docker Hub
type dockerHubRepo struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Description    string   `json:"description"`
	IsPrivate      bool     `json:"is_private"`
	StarCount      int      `json:"star_count"`
	PullCount      int      `json:"pull_count"`
	LastUpdated    string   `json:"last_updated"`
	ContentTypes   []string `json:"content_types"`
	RepositoryType string   `json:"repository_type"`
	IsAutomated    bool     `json:"is_automated"`
	CanEdit        bool     `json:"can_edit"`
	IsMigrated     bool     `json:"is_migrated"`
	Affiliation    string   `json:"affiliation"`
	HubUser        string   `json:"hub_user"`
	NamespaceType  string   `json:"namespace_type"`
}

// Name returns the name of this search source
func (c *DockerHubClient) Name() string {
	return DockerHubSourceName
}

// Search searches for models on Docker Hub in the ai/ namespace
func (c *DockerHubClient) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 32
	}

	var results []SearchResult
	query := strings.ToLower(opts.Query)
	nextURL := ""

	// Docker Hub API paginates at 100 results max per page
	pageSize := 100
	if limit < pageSize {
		pageSize = limit
	}

	for len(results) < limit {
		var fullURL string
		if nextURL != "" {
			fullURL = nextURL
		} else {
			// Build the URL for listing repositories in the ai/ namespace
			apiURL := fmt.Sprintf("%s/v2/repositories/%s/", c.baseURL, dockerHubAIOrg)
			params := url.Values{}
			params.Set("page_size", fmt.Sprintf("%d", pageSize))
			params.Set("ordering", "-pull_count") // Sort by popularity
			fullURL = apiURL + "?" + params.Encode()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching from Docker Hub: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("rate limited by Docker Hub, please try again later")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status from Docker Hub: %s", resp.Status)
		}

		var response dockerHubRepoListResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		for _, repo := range response.Results {
			// Skip private repos
			if repo.IsPrivate {
				continue
			}

			// Apply client-side filtering if query is provided
			if query != "" {
				nameMatch := strings.Contains(strings.ToLower(repo.Name), query)
				descMatch := strings.Contains(strings.ToLower(repo.Description), query)
				if !nameMatch && !descMatch {
					continue
				}
			}

			// Determine backend type from name and description
			backend := determineDockerHubBackend(repo.Name, repo.Description)

			results = append(results, SearchResult{
				Name:        fmt.Sprintf("%s/%s", repo.Namespace, repo.Name),
				Description: truncateString(repo.Description, 50),
				Downloads:   int64(repo.PullCount),
				Stars:       int64(repo.StarCount),
				Source:      DockerHubSourceName,
				Official:    repo.Namespace == dockerHubAIOrg,
				UpdatedAt:   repo.LastUpdated,
				Backend:     backend,
			})

			if len(results) >= limit {
				break
			}
		}

		// Check if there are more pages
		if response.Next == "" || len(results) >= limit {
			break
		}
		nextURL = response.Next
	}

	return results, nil
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// determineDockerHubBackend determines the backend type from model name and description
func determineDockerHubBackend(name, description string) string {
	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(description)
	combined := nameLower + " " + descLower

	var hasVLLM, hasLlamaCpp bool

	// Check for vLLM indicators
	if strings.Contains(combined, "vllm") || strings.Contains(combined, "safetensors") {
		hasVLLM = true
	}

	// Check for llama.cpp indicators (gguf is the format used by llama.cpp)
	if strings.Contains(combined, "llama.cpp") || strings.Contains(combined, "llamacpp") ||
		strings.Contains(combined, "gguf") || strings.Contains(combined, "llama-cpp") {
		hasLlamaCpp = true
	}

	if hasVLLM && hasLlamaCpp {
		return "llama.cpp, vllm"
	}
	if hasVLLM {
		return "vllm"
	}
	if hasLlamaCpp {
		return "llama.cpp"
	}

	// Default to llama.cpp for ai/ namespace models as they primarily use GGUF format
	return "llama.cpp"
}
