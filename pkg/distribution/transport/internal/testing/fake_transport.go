// Package testing provides common test utilities for transport packages.
package testing

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// FakeResource represents a resource that can be served by FakeTransport.
type FakeResource struct {
	// Data provides random access to the resource content.
	Data io.ReaderAt
	// Length is the total number of bytes in the resource content.
	Length int64
	// SupportsRange indicates if this resource supports byte ranges.
	SupportsRange bool
	// ETag is the ETag header value (optional).
	ETag string
	// LastModified is the Last-Modified header value (optional).
	LastModified string
	// ContentType is the Content-Type header value (optional).
	ContentType string
	// Headers are additional headers to include in responses.
	Headers http.Header
}

// FakeTransport is a test http.RoundTripper that serves fake resources.
type FakeTransport struct {
	mu        sync.Mutex
	resources map[string]*FakeResource
	requests  []http.Request
	// FailAfter causes the transport to fail after serving this many bytes
	// on a request (for simulating connection failures).
	failAfter map[string]int
	// failCount tracks how many times we've failed for each URL.
	failCount map[string]int
	// RequestHook is called for each request if set.
	RequestHook func(*http.Request)
	// ResponseHook is called for each response if set.
	ResponseHook func(*http.Response)
}

// NewFakeTransport creates a new FakeTransport.
func NewFakeTransport() *FakeTransport {
	return &FakeTransport{
		resources: make(map[string]*FakeResource),
		failAfter: make(map[string]int),
		failCount: make(map[string]int),
	}
}

// Add adds a resource to the fake transport.
func (ft *FakeTransport) Add(url string, resource *FakeResource) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.resources[url] = resource
}

// AddSimple adds a simple resource with the provided reader and length.
func (ft *FakeTransport) AddSimple(url string, data io.ReaderAt, length int64, supportsRange bool) {
	ft.Add(url, &FakeResource{
		Data:          data,
		Length:        length,
		SupportsRange: supportsRange,
	})
}

// SetFailAfter configures the transport to fail after serving n bytes for
// the given URL.
func (ft *FakeTransport) SetFailAfter(url string, n int) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.failAfter[url] = n
}

// GetRequests returns a copy of all requests made to this transport.
func (ft *FakeTransport) GetRequests() []http.Request {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	reqs := make([]http.Request, len(ft.requests))
	copy(reqs, ft.requests)
	return reqs
}

// GetRequestHeaders returns the headers from all requests for a given URL.
func (ft *FakeTransport) GetRequestHeaders(url string) []http.Header {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	var headers []http.Header
	for _, req := range ft.requests {
		if req.URL.String() == url {
			h := make(http.Header)
			for k, v := range req.Header {
				h[k] = append([]string(nil), v...)
			}
			headers = append(headers, h)
		}
	}
	return headers
}

// RoundTrip implements http.RoundTripper.
func (ft *FakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.mu.Lock()
	// Store request
	reqCopy := *req
	if req.Header != nil {
		reqCopy.Header = req.Header.Clone()
	}
	ft.requests = append(ft.requests, reqCopy)

	// Get resource
	resource, exists := ft.resources[req.URL.String()]
	failAfter := ft.failAfter[req.URL.String()]
	ft.mu.Unlock()

	if ft.RequestHook != nil {
		ft.RequestHook(req)
	}

	if !exists {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}

	// Handle HEAD request
	if req.Method == http.MethodHead {
		resp := ft.createResponse(req, resource, nil, http.StatusOK)
		if ft.ResponseHook != nil {
			ft.ResponseHook(resp)
		}
		return resp, nil
	}

	// Handle Range request
	if rangeHeader := req.Header.Get("Range"); rangeHeader != "" && resource.SupportsRange {
		return ft.handleRangeRequest(req, resource, rangeHeader, failAfter)
	}

	// Regular GET request
	var body io.ReadCloser
	if failAfter > 0 && ft.getFailCount(req.URL.String()) == 0 {
		// First request - fail after specified bytes
		body = NewFlakyReader(resource.Data, resource.Length, failAfter)
		ft.incrementFailCount(req.URL.String())
	} else {
		// Subsequent request or no failure configured
		body = io.NopCloser(io.NewSectionReader(resource.Data, 0, resource.Length))
	}

	resp := ft.createResponse(req, resource, body, http.StatusOK)
	if ft.ResponseHook != nil {
		ft.ResponseHook(resp)
	}
	return resp, nil
}

// handleRangeRequest serves a single byte range request for a resource.
// It validates the Range and If-Range headers and returns either 206 with the
// requested slice, or 200 with the full resource if validation fails.
// Multi-range specifications are not supported and result in 400.
func (ft *FakeTransport) handleRangeRequest(req *http.Request, resource *FakeResource, rangeHeader string, failAfter int) (*http.Response, error) {
	// Parse range header (simplified - only handles single ranges)
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return ft.createErrorResponse(req, http.StatusBadRequest), nil
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return ft.createErrorResponse(req, http.StatusBadRequest), nil
	}

	var start, end int64
	var err error

	if parts[0] != "" {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return ft.createErrorResponse(req, http.StatusBadRequest), nil
		}
	}

	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return ft.createErrorResponse(req, http.StatusBadRequest), nil
		}
	} else {
		end = resource.Length - 1
	}

	// Validate range
	if start < 0 || end >= resource.Length || start > end {
		resp := ft.createErrorResponse(req, http.StatusRequestedRangeNotSatisfiable)
		resp.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", resource.Length))
		if ft.ResponseHook != nil {
			ft.ResponseHook(resp)
		}
		return resp, nil
	}

	// Check If-Range
	if ifRange := req.Header.Get("If-Range"); ifRange != "" {
		// Check if If-Range matches either ETag or Last-Modified
		matches := false

		// Only match strong ETags for If-Range
		if resource.ETag != "" && !strings.HasPrefix(resource.ETag, "W/") {
			if ifRange == resource.ETag {
				matches = true
			}
		}

		// Also check Last-Modified
		if !matches && resource.LastModified != "" {
			if ifRange == resource.LastModified {
				matches = true
			}
		}

		if !matches {
			// Validator doesn't match - return full content
			body := NewFlakyReader(resource.Data, resource.Length, failAfter)
			resp := ft.createResponse(req, resource, body, http.StatusOK)
			if ft.ResponseHook != nil {
				ft.ResponseHook(resp)
			}
			return resp, nil
		}
	}

	// Serve range
	body := io.NopCloser(io.NewSectionReader(resource.Data, start, end-start+1))

	resp := ft.createResponse(req, resource, body, http.StatusPartialContent)
	resp.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, resource.Length))
	resp.ContentLength = end - start + 1

	if ft.ResponseHook != nil {
		ft.ResponseHook(resp)
	}
	return resp, nil
}

// createResponse builds a basic http.Response for the given resource and
// status code, copying standard headers and any optional metadata.
func (ft *FakeTransport) createResponse(req *http.Request, resource *FakeResource, body io.ReadCloser, statusCode int) *http.Response {
	if body == nil {
		body = io.NopCloser(bytes.NewReader(nil))
	}

	resp := &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       body,
		Request:    req,
	}

	// Set standard headers
	if resource.SupportsRange {
		resp.Header.Set("Accept-Ranges", "bytes")
	}

	if resource.ETag != "" {
		resp.Header.Set("ETag", resource.ETag)
	}

	if resource.LastModified != "" {
		resp.Header.Set("Last-Modified", resource.LastModified)
	}

	if resource.ContentType != "" {
		resp.Header.Set("Content-Type", resource.ContentType)
	}

	// Copy additional headers
	if resource.Headers != nil {
		for k, v := range resource.Headers {
			resp.Header[k] = v
		}
	}

	// Set Content-Length
	if statusCode == http.StatusOK {
		resp.ContentLength = resource.Length
		resp.Header.Set("Content-Length", strconv.FormatInt(resource.Length, 10))
	}

	return resp
}

// createErrorResponse constructs a minimal error response with the provided
// status code and an empty body.
func (ft *FakeTransport) createErrorResponse(req *http.Request, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
	}
}

// getFailCount returns how many failures have been injected for the URL so
// far. It is safe for concurrent use.
func (ft *FakeTransport) getFailCount(url string) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.failCount[url]
}

// incrementFailCount increments the injected failure counter for the URL.
// It is safe for concurrent use.
func (ft *FakeTransport) incrementFailCount(url string) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.failCount[url]++
}
