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
	huggingFaceAPIURL = "https://huggingface.co/api"
)

// HuggingFaceClient searches for models on HuggingFace Hub
type HuggingFaceClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewHuggingFaceClient creates a new HuggingFace search client
func NewHuggingFaceClient() *HuggingFaceClient {
	return &HuggingFaceClient{
		httpClient: NewHTTPClient(),
		baseURL:    huggingFaceAPIURL,
	}
}

// huggingFaceModel represents a model from the HuggingFace API
type huggingFaceModel struct {
	ID          string   `json:"id"`
	ModelID     string   `json:"modelId"`
	Likes       int      `json:"likes"`
	Downloads   int      `json:"downloads"`
	Tags        []string `json:"tags"`
	PipelineTag string   `json:"pipeline_tag,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	Private     bool     `json:"private"`
}

// Name returns the name of this search source
func (c *HuggingFaceClient) Name() string {
	return HuggingFaceSourceName
}

// Search searches for llama.cpp and vLLM compatible models on HuggingFace
func (c *HuggingFaceClient) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 32
	}
	// HuggingFace API supports up to 1000 results in a single request
	if limit > 1000 {
		limit = 1000
	}

	// Build the URL for searching llama.cpp and vLLM compatible models
	apiURL := fmt.Sprintf("%s/models", c.baseURL)
	params := url.Values{}
	params.Set("apps", "vllm,llama.cpp")
	params.Set("sort", "downloads")
	params.Set("direction", "-1")
	params.Set("limit", fmt.Sprintf("%d", limit))

	if opts.Query != "" {
		params.Set("search", opts.Query)
	}

	fullURL := apiURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching from HuggingFace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by HuggingFace, please try again later")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from HuggingFace: %s", resp.Status)
	}

	var models []huggingFaceModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var results []SearchResult
	for _, model := range models {
		// Skip private models
		if model.Private {
			continue
		}

		// Use modelId if available, otherwise use id
		modelName := model.ModelID
		if modelName == "" {
			modelName = model.ID
		}

		// Generate description from tags
		description := generateDescription(model.Tags, model.PipelineTag)

		// Determine backend type from tags
		backend := determineBackend(model.Tags)

		results = append(results, SearchResult{
			Name:        modelName,
			Description: truncateString(description, 50),
			Downloads:   int64(model.Downloads),
			Stars:       int64(model.Likes),
			Source:      HuggingFaceSourceName,
			Official:    false,
			UpdatedAt:   model.CreatedAt,
			Backend:     backend,
		})
	}

	return results, nil
}

// generateDescription creates a description from model tags
func generateDescription(tags []string, pipelineTag string) string {
	var parts []string

	if pipelineTag != "" {
		parts = append(parts, pipelineTag)
	}

	// Look for interesting tags (skip generic ones)
	skipTags := map[string]bool{
		"gguf": true, "transformers": true, "pytorch": true,
		"safetensors": true, "license:apache-2.0": true,
	}

	for _, tag := range tags {
		tag = strings.ToLower(tag)
		if skipTags[tag] {
			continue
		}
		// Include architecture/model type tags
		if strings.HasPrefix(tag, "llama") ||
			strings.HasPrefix(tag, "mistral") ||
			strings.HasPrefix(tag, "phi") ||
			strings.HasPrefix(tag, "qwen") ||
			strings.Contains(tag, "instruct") ||
			strings.Contains(tag, "chat") {
			parts = append(parts, tag)
			if len(parts) >= 3 {
				break
			}
		}
	}

	if len(parts) == 0 {
		return "AI model"
	}
	return strings.Join(parts, ", ")
}

// determineBackend determines the backend type from HuggingFace model tags.
// Since we filter by apps=vllm,llama.cpp, all results are compatible with at least one backend.
// - GGUF format models work with llama.cpp
// - Transformers/safetensors models work with vLLM
func determineBackend(tags []string) string {
	var hasVLLM, hasLlamaCpp bool

	for _, tag := range tags {
		tagLower := strings.ToLower(tag)

		// Check for explicit vllm tag or formats that indicate vLLM compatibility
		if tagLower == "vllm" || tagLower == "text-generation-inference" {
			hasVLLM = true
		}
		// Transformers/safetensors models are typically vLLM compatible
		if tagLower == "transformers" || tagLower == "safetensors" {
			hasVLLM = true
		}

		// Check for llama.cpp compatibility (GGUF format)
		if tagLower == "llama.cpp" || tagLower == "llama-cpp" || tagLower == "gguf" {
			hasLlamaCpp = true
		}
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
	// Fallback: since we filter by apps=vllm,llama.cpp, model must be compatible with one
	// but we couldn't determine which from tags
	return "llama.cpp"
}
