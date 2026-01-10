package search

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
)

// SourceType represents the source to search
type SourceType string

const (
	SourceAll         SourceType = "all"
	SourceDockerHub   SourceType = "dockerhub"
	SourceHuggingFace SourceType = "huggingface"
)

// AggregatedClient searches multiple sources and merges results
type AggregatedClient struct {
	clients []SearchClient
	errOut  io.Writer
}

// NewAggregatedClient creates a client that searches the specified sources
func NewAggregatedClient(source SourceType, errOut io.Writer) *AggregatedClient {
	var clients []SearchClient

	switch source {
	case SourceDockerHub:
		clients = []SearchClient{NewDockerHubClient()}
	case SourceHuggingFace:
		clients = []SearchClient{NewHuggingFaceClient()}
	case SourceAll:
		clients = []SearchClient{
			NewDockerHubClient(),
			NewHuggingFaceClient(),
		}
	default: // This handles any unexpected values
		clients = []SearchClient{
			NewDockerHubClient(),
			NewHuggingFaceClient(),
		}
	}

	return &AggregatedClient{
		clients: clients,
		errOut:  errOut,
	}
}

// searchResult holds results from a single source along with any error
type searchResult struct {
	results []SearchResult
	err     error
	source  string
}

// Search searches all configured sources and merges results
func (c *AggregatedClient) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	// Search all sources concurrently
	resultsChan := make(chan searchResult, len(c.clients))
	var wg sync.WaitGroup

	for _, client := range c.clients {
		wg.Add(1)
		go func(client SearchClient) {
			defer wg.Done()
			results, err := client.Search(ctx, opts)
			resultsChan <- searchResult{
				results: results,
				err:     err,
				source:  client.Name(),
			}
		}(client)
	}

	// Wait for all searches to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var allResults []SearchResult
	var errors []error

	for result := range resultsChan {
		if result.err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", result.source, result.err))
			if c.errOut != nil {
				fmt.Fprintf(c.errOut, "Warning: failed to search %s: %v\n", result.source, result.err)
			}
			continue
		}
		allResults = append(allResults, result.results...)
	}

	// If all sources failed, return the collected errors
	if len(allResults) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all search sources failed: %v", errors)
	}

	// Sort by source (Docker Hub first), then by downloads within each source
	sort.Slice(allResults, func(i, j int) bool {
		// Docker Hub comes before HuggingFace
		if allResults[i].Source != allResults[j].Source {
			return allResults[i].Source == DockerHubSourceName
		}
		// Within same source, sort by downloads (popularity)
		return allResults[i].Downloads > allResults[j].Downloads
	})

	// Limit total results if needed
	if opts.Limit > 0 && len(allResults) > opts.Limit {
		allResults = allResults[:opts.Limit]
	}

	return allResults, nil
}

// ParseSource parses a source string into a SourceType
func ParseSource(s string) (SourceType, error) {
	switch s {
	case "all", "":
		return SourceAll, nil
	case "dockerhub", "docker", "hub":
		return SourceDockerHub, nil
	case "huggingface", "hf":
		return SourceHuggingFace, nil
	default:
		return "", fmt.Errorf("unknown source %q: valid options are 'all', 'dockerhub', 'docker', 'hub', 'huggingface', 'hf'", s)
	}
}
