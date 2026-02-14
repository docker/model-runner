package huggingface

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
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

type LFSBatchRequest struct {
	Operation string           `json:"operation"`
	Transfers []string         `json:"transfers,omitempty"`
	Objects   []LFSBatchObject `json:"objects"`
}

type LFSBatchObject struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

type LFSObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type LFSAction struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header,omitempty"`
}

type LFSObject struct {
	OID     string               `json:"oid"`
	Size    int64                `json:"size"`
	Actions map[string]LFSAction `json:"actions,omitempty"`
	Error   *LFSObjectError      `json:"error,omitempty"`
}

type LFSBatchResponse struct {
	Transfer string      `json:"transfer,omitempty"`
	Objects  []LFSObject `json:"objects"`
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

// RepoInfo contains metadata about a HuggingFace repository
type RepoInfo struct {
	LastModified time.Time `json:"lastModified"`
}

// GetRepoInfo fetches repository metadata from the HuggingFace API.
// This returns information such as the last modified timestamp, which is useful
// for producing deterministic OCI digests.
func (c *Client) GetRepoInfo(ctx context.Context, repo, revision string) (*RepoInfo, error) {
	if revision == "" {
		revision = "main"
	}

	reqURL := fmt.Sprintf("%s/api/models/%s/revision/%s", c.baseURL, repo, url.PathEscape(revision))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get repo info: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp, repo); err != nil {
		return nil, err
	}

	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &info, nil
}

type CommitFile struct {
	RepoPath string
	Content  io.Reader
}

type LFSCommitFile struct {
	Path string `json:"path"`
	Algo string `json:"algo"`
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

type ndjsonEntry struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// CreateCommit creates a commit in a HuggingFace repository using the NDJSON API.
// LFS files must be pre-uploaded via LFSBatch + UploadLFSObject before calling this.
// Small files are sent inline as base64.
func (c *Client) CreateCommit(ctx context.Context, repo, message string, directFiles []CommitFile, lfsFiles []LFSCommitFile) error {
	if repo == "" {
		return fmt.Errorf("repository is required")
	}

	endpoint := fmt.Sprintf("%s/api/models/%s/commit/main", c.baseURL, escapePath(repo))

	var buf bytes.Buffer

	headerEntry := ndjsonEntry{
		Key: "header",
		Value: map[string]interface{}{
			"summary":     message,
			"description": "",
		},
	}
	if err := json.NewEncoder(&buf).Encode(headerEntry); err != nil {
		return fmt.Errorf("encode commit header: %w", err)
	}

	for _, lf := range lfsFiles {
		entry := ndjsonEntry{
			Key: "lfsFile",
			Value: map[string]interface{}{
				"path": lf.Path,
				"algo": lf.Algo,
				"oid":  lf.OID,
				"size": lf.Size,
			},
		}
		if err := json.NewEncoder(&buf).Encode(entry); err != nil {
			return fmt.Errorf("encode lfs file entry %s: %w", lf.Path, err)
		}
	}

	for _, f := range directFiles {
		data, err := io.ReadAll(f.Content)
		if err != nil {
			return fmt.Errorf("read file %s: %w", f.RepoPath, err)
		}
		entry := ndjsonEntry{
			Key: "file",
			Value: map[string]interface{}{
				"path":     f.RepoPath,
				"encoding": "base64",
				"content":  base64.StdEncoding.EncodeToString(data),
			},
		}
		if err := json.NewEncoder(&buf).Encode(entry); err != nil {
			return fmt.Errorf("encode file entry %s: %w", f.RepoPath, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create commit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkUploadResponse(resp, repo); err != nil {
		return err
	}

	return nil
}

func (c *Client) LFSBatch(ctx context.Context, repo string, objects []LFSBatchObject) (*LFSBatchResponse, error) {
	if repo == "" {
		return nil, fmt.Errorf("repository is required")
	}
	if len(objects) == 0 {
		return &LFSBatchResponse{}, nil
	}

	endpoint := fmt.Sprintf("%s/%s.git/info/lfs/objects/batch", c.baseURL, escapePath(repo))
	reqBody := LFSBatchRequest{
		Operation: "upload",
		Transfers: []string{"basic"},
		Objects:   objects,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encode lfs batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lfs batch: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp, repo); err != nil {
		return nil, err
	}

	var batchResp LFSBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return nil, fmt.Errorf("decode lfs batch response: %w", err)
	}

	return &batchResp, nil
}

func (c *Client) UploadLFSObject(ctx context.Context, action LFSAction, content io.Reader, size int64) error {
	if action.Href == "" {
		return fmt.Errorf("upload action href is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, action.Href, content)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}

	for key, value := range action.Header {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if size > 0 {
		req.ContentLength = size
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload lfs object: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkUploadResponse(resp, ""); err != nil {
		return err
	}

	return nil
}

func (c *Client) VerifyLFSObject(ctx context.Context, action LFSAction, oid string, size int64) error {
	if action.Href == "" {
		return nil
	}
	data, err := json.Marshal(LFSBatchObject{OID: oid, Size: size})
	if err != nil {
		return fmt.Errorf("encode verify request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, action.Href, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create verify request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	for key, value := range action.Header {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("verify lfs object: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkUploadResponse(resp, ""); err != nil {
		return err
	}

	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

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

func (c *Client) checkUploadResponse(resp *http.Response, repo string) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &AuthError{Repo: repo, StatusCode: resp.StatusCode}
		case http.StatusNotFound:
			return &NotFoundError{Repo: repo}
		default:
			return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(body))
		}
	}
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
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
