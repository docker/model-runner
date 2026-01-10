package search

import "context"

// Constants for source names
const (
	DockerHubSourceName   = "Docker Hub"
	HuggingFaceSourceName = "HuggingFace"
)

// SearchResult represents a model found during search
type SearchResult struct {
	Name        string // Full model reference (e.g., "ai/llama3.2" or "hf.co/org/model")
	Description string // Short description
	Downloads   int64  // Download/pull count
	Stars       int64  // Star/like count
	Source      string // "Docker Hub" or "HuggingFace"
	Official    bool   // Whether this is an official model
	UpdatedAt   string // Last update timestamp
	Backend     string // Backend type: "llama.cpp", "vllm", or "llama.cpp, vllm" if both
}

// SearchOptions configures the search behavior
type SearchOptions struct {
	Query string // Search term (empty = list all)
	Limit int    // Maximum results per source; aggregated clients may also apply this as a global cap after merging results
}

// SearchClient defines the interface for searching a model registry
type SearchClient interface {
	Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error)
	Name() string
}
