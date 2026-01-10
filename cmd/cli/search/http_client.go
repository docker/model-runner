package search

import (
	"net/http"
	"time"
)

// NewHTTPClient creates a new HTTP client with standard configuration
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}
