package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

const (
	defaultBaseURL   = "https://huggingface.co"
	defaultUserAgent = "model-distribution"
)

// Client handles HuggingFace Hub API interactions
type Client struct {
	httpClient *http.Client
	userAgent  string
	token      string
	baseURL    string
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithToken sets the HuggingFace API token for authentication
func WithToken(token string) ClientOption {
	return func(c *Client) {
		if token != "" {
			c.token = token
		}
	}
}

// WithTransport sets the HTTP transport for the client
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(c *Client) {
		if transport != nil {
			c.httpClient.Transport = transport
		}
	}
}

// WithUserAgent sets the User-Agent header for requests
func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

// WithBaseURL sets a custom base URL (useful for testing)
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimSuffix(baseURL, "/")
		}
	}
}

// NewClient creates a new HuggingFace Hub API client
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{},
		userAgent:  defaultUserAgent,
		baseURL:    defaultBaseURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ListFiles returns all files in a repository at a given revision, recursively traversing all directories
func (c *Client) ListFiles(ctx context.Context, repo, revision string) ([]RepoFile, error) {
	if revision == "" {
		revision = "main"
	}

	return c.listFilesRecursive(ctx, repo, revision, "")
}

// listFilesRecursive recursively lists all files starting from the given path
func (c *Client) listFilesRecursive(ctx context.Context, repo, revision, filePath string) ([]RepoFile, error) {
	entries, err := c.ListFilesInPath(ctx, repo, revision, filePath)
	if err != nil {
		return nil, err
	}

	var allFiles []RepoFile
	for _, entry := range entries {
		switch entry.Type {
		case "file":
			allFiles = append(allFiles, entry)
		case "directory":
			// Recursively list files in subdirectory
			subFiles, err := c.listFilesRecursive(ctx, repo, revision, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("list files in %s: %w", entry.Path, err)
			}
			allFiles = append(allFiles, subFiles...)
		}
	}

	return allFiles, nil
}

// ListFilesInPath returns files and directories at a specific path in the repository
func (c *Client) ListFilesInPath(ctx context.Context, repo, revision, filePath string) ([]RepoFile, error) {
	if revision == "" {
		revision = "main"
	}

	// HuggingFace API endpoint for listing files
	endpointPath := path.Join(revision, filePath)
	url := fmt.Sprintf("%s/api/models/%s/tree/%s", c.baseURL, repo, endpointPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp, repo); err != nil {
		return nil, err
	}

	var files []RepoFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return files, nil
}

// DownloadFile streams a file from the repository
// Returns the reader, content length (-1 if unknown), and any error
func (c *Client) DownloadFile(ctx context.Context, repo, revision, filename string) (io.ReadCloser, int64, error) {
	if revision == "" {
		revision = "main"
	}

	// HuggingFace file download endpoint (handles LFS redirects automatically)
	url := fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repo, revision, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("download file: %w", err)
	}

	if err := c.checkResponse(resp, repo); err != nil {
		resp.Body.Close()
		return nil, 0, err
	}

	return resp.Body, resp.ContentLength, nil
}

// setHeaders sets common headers for HuggingFace API requests
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// checkResponse checks the HTTP response for errors
func (c *Client) checkResponse(resp *http.Response, repo string) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthError{Repo: repo, StatusCode: resp.StatusCode}
	case http.StatusNotFound:
		return &NotFoundError{Repo: repo}
	case http.StatusTooManyRequests:
		return &RateLimitError{Repo: repo}
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

// AuthError indicates authentication failure
type AuthError struct {
	Repo       string
	StatusCode int
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication required for repository %q (status %d)", e.Repo, e.StatusCode)
}

// NotFoundError indicates the repository or file was not found
type NotFoundError struct {
	Repo string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("repository %q not found", e.Repo)
}

// RateLimitError indicates rate limiting
type RateLimitError struct {
	Repo string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited while accessing repository %q", e.Repo)
}
