package remote

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRangeTransport_RedirectPreservesRangeHeader verifies that when a
// registry redirects a blob download to a CDN, the Range header is
// preserved on the redirect request. Without this, resumable downloads
// restart from byte 0 after every interruption.
func TestRangeTransport_RedirectPreservesRangeHeader(t *testing.T) {
	const blobContent = "0123456789abcdef" // 16 bytes
	const resumeOffset = 10
	const blobDigest = "sha256:deadbeef"

	var cdnRangeHeader string

	// CDN server: serves partial content if Range header is present.
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnRangeHeader = r.Header.Get("Range")
		if cdnRangeHeader != "" {
			// Parse the range start
			var start int64
			if _, err := fmt.Sscanf(cdnRangeHeader, "bytes=%d-", &start); err == nil {
				partial := blobContent[start:]
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(blobContent)-1, len(blobContent)))
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(partial)))
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(partial))
				return
			}
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blobContent)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(blobContent))
	}))
	defer cdn.Close()

	// Registry server: redirects blob requests to CDN.
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/") {
			http.Redirect(w, r, cdn.URL+"/blob-content", http.StatusFound)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer registry.Close()

	// Set up resume offsets in context
	offsets := map[string]int64{blobDigest: resumeOffset}
	rs := &RangeSuccess{}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, rs)

	// Create rangeTransport wrapping the default transport
	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		// Disable client-level redirects; rangeTransport handles them.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Make the blob request through the registry (which will redirect to CDN)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registry.URL+"/v2/test/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Verify the CDN received the Range header
	expectedRange := fmt.Sprintf("bytes=%d-", resumeOffset)
	if cdnRangeHeader != expectedRange {
		t.Errorf("CDN Range header = %q, want %q", cdnRangeHeader, expectedRange)
	}

	// Verify we got partial content
	if resp.StatusCode != http.StatusPartialContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}

	// Verify the body is the partial content
	expected := blobContent[resumeOffset:]
	if string(body) != expected {
		t.Errorf("body = %q, want %q", string(body), expected)
	}

	// Verify RangeSuccess was recorded
	if offset, ok := rs.Get(blobDigest); !ok || offset != resumeOffset {
		t.Errorf("RangeSuccess.Get(%q) = (%d, %v), want (%d, true)", blobDigest, offset, ok, resumeOffset)
	}
}

// TestRangeTransport_RedirectStripsAuthOnCrossDomain verifies that the
// Authorization header is stripped when following a redirect to a different
// host, matching Go's http.Client security policy.
func TestRangeTransport_RedirectStripsAuthOnCrossDomain(t *testing.T) {
	const blobDigest = "sha256:deadbeef"
	const resumeOffset = 100

	var cdnAuthHeader string

	// CDN server on a different "host"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", resumeOffset, 200, 200))
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("partial"))
	}))
	defer cdn.Close()

	// Registry: redirects to CDN (different host)
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/blob-data", http.StatusFound)
	}))
	defer registry.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, &RangeSuccess{})

	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registry.URL+"/v2/repo/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// The CDN is on a different host, so Authorization must be stripped.
	if cdnAuthHeader != "" {
		t.Errorf("CDN received Authorization header %q; want empty (should be stripped on cross-domain redirect)", cdnAuthHeader)
	}
}

// TestRangeTransport_NoRedirectHandlingWithoutRange verifies that the
// transport-level redirect logic does NOT activate when no Range header is
// set. Non-resume requests should let Go's http.Client handle redirects as
// usual.
func TestRangeTransport_NoRedirectHandlingWithoutRange(t *testing.T) {
	var cdnHit bool

	// CDN server
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnHit = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("full content"))
	}))
	defer cdn.Close()

	// Registry: redirects blob requests to CDN
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/blob", http.StatusFound)
	}))
	defer registry.Close()

	// No resume offsets — this is a fresh download
	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		// Disable client-level redirects to verify transport doesn't follow them
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, registry.URL+"/v2/repo/blobs/sha256:abc123", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Without Range, the transport should NOT follow the redirect.
	// We should get the 302 back since CheckRedirect returns ErrUseLastResponse.
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want %d (transport should not follow redirects without Range)", resp.StatusCode, http.StatusFound)
	}
	if cdnHit {
		t.Error("CDN was hit but should not have been — transport should not follow redirects for non-resume requests")
	}
}

// TestRangeTransport_MultipleRedirects verifies that the transport follows
// a chain of redirects (e.g., registry → CDN1 → CDN2).
func TestRangeTransport_MultipleRedirects(t *testing.T) {
	const blobDigest = "sha256:multiredirect"
	const resumeOffset int64 = 50
	const blobContent = "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789" // 100 bytes

	var finalRangeHeader string

	// Final CDN server
	finalCDN := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalRangeHeader = r.Header.Get("Range")
		partial := blobContent[resumeOffset:]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", resumeOffset, len(blobContent)-1, len(blobContent)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(partial))
	}))
	defer finalCDN.Close()

	// Intermediate CDN: redirects to final CDN
	intermediateCDN := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalCDN.URL+"/final-blob", http.StatusTemporaryRedirect)
	}))
	defer intermediateCDN.Close()

	// Registry: redirects to intermediate CDN
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, intermediateCDN.URL+"/intermediate-blob", http.StatusFound)
	}))
	defer registry.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	rs := &RangeSuccess{}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, rs)

	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registry.URL+"/v2/test/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Range header must reach the final CDN
	expectedRange := fmt.Sprintf("bytes=%d-", resumeOffset)
	if finalRangeHeader != expectedRange {
		t.Errorf("final CDN Range header = %q, want %q", finalRangeHeader, expectedRange)
	}

	if resp.StatusCode != http.StatusPartialContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}

	if offset, ok := rs.Get(blobDigest); !ok || offset != resumeOffset {
		t.Errorf("RangeSuccess.Get(%q) = (%d, %v), want (%d, true)", blobDigest, offset, ok, resumeOffset)
	}
}

// TestRangeTransport_ServerIgnoresRange verifies that when the server (or
// CDN after redirect) ignores the Range header and returns 200, the
// RangeSuccess tracker is NOT updated — preventing corrupt blobs from
// appending a full response to an existing partial file.
func TestRangeTransport_ServerIgnoresRange(t *testing.T) {
	const blobDigest = "sha256:norange"
	const blobContent = "full content from byte 0"
	const resumeOffset int64 = 100

	// CDN: ignores Range header, returns full content
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(blobContent))
	}))
	defer cdn.Close()

	// Registry: redirects to CDN
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/blob", http.StatusFound)
	}))
	defer registry.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	rs := &RangeSuccess{}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, rs)

	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registry.URL+"/v2/repo/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Server returned 200 (not 206), so RangeSuccess must NOT be recorded.
	if _, ok := rs.Get(blobDigest); ok {
		t.Error("RangeSuccess was recorded but server returned 200 (not 206); resume would produce a corrupt blob")
	}
}

// TestRangeTransport_RedirectPreservesAuthOnSameHost verifies that the
// Authorization header is preserved when following a redirect to the same
// host (only the path changes).
func TestRangeTransport_RedirectPreservesAuthOnSameHost(t *testing.T) {
	const blobDigest = "sha256:samehost"
	const resumeOffset = 100

	var redirectedAuthHeader string

	// Single server that redirects on the first path and serves on the second.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/repo/blobs/"):
			// Redirect to a different path on the same host.
			http.Redirect(w, r, "/v2/redirected/blobs/"+blobDigest, http.StatusTemporaryRedirect)
		case strings.HasPrefix(r.URL.Path, "/v2/redirected/blobs/"):
			redirectedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", resumeOffset, 200, 200))
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte("partial"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, &RangeSuccess{})

	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v2/repo/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer my-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Same host → Authorization must be preserved.
	if redirectedAuthHeader != "Bearer my-token" {
		t.Errorf("redirected Authorization = %q, want %q (should be preserved on same-host redirect)", redirectedAuthHeader, "Bearer my-token")
	}
}

// TestRangeTransport_RedirectStripsSensitiveOnSchemeDowngrade verifies that
// sensitive headers are stripped when redirecting from HTTPS to HTTP.
func TestRangeTransport_RedirectStripsSensitiveOnSchemeDowngrade(t *testing.T) {
	const blobDigest = "sha256:downgrade"
	const resumeOffset = 100

	var cdnCookieHeader string

	// HTTP server (simulates scheme downgrade target).
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnCookieHeader = r.Header.Get("Cookie")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", resumeOffset, 200, 200))
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("partial"))
	}))
	defer cdn.Close()

	// HTTPS server that redirects to the HTTP cdn.
	registry := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to plain HTTP (scheme downgrade).
		http.Redirect(w, r, cdn.URL+"/blob", http.StatusFound)
	}))
	defer registry.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	ctx := WithResumeOffsets(t.Context(), offsets)
	ctx = WithRangeSuccess(ctx, &RangeSuccess{})

	transport := &rangeTransport{base: registry.Client().Transport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registry.URL+"/v2/repo/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", "session=secret")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// HTTPS→HTTP scheme downgrade must strip sensitive headers.
	if cdnCookieHeader != "" {
		t.Errorf("CDN received Cookie header %q; want empty (should be stripped on scheme downgrade)", cdnCookieHeader)
	}
}

// TestRangeTransport_MaxRedirectsExceeded verifies that an explicit error
// is returned when the redirect limit is hit.
func TestRangeTransport_MaxRedirectsExceeded(t *testing.T) {
	const blobDigest = "sha256:loopdigest"
	const resumeOffset int64 = 50

	// Server that always redirects to itself (infinite redirect loop).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer srv.Close()

	offsets := map[string]int64{blobDigest: resumeOffset}
	ctx := WithResumeOffsets(t.Context(), offsets)

	transport := &rangeTransport{base: http.DefaultTransport}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v2/repo/blobs/"+blobDigest, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected error for max redirects exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("error = %q, want it to mention redirect limit", err.Error())
	}
}

// TestIsRedirect covers all redirect status codes.
func TestIsRedirect(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{204, false},
		{206, false},
		{301, true},
		{302, true},
		{303, true},
		{304, false},
		{307, true},
		{308, true},
		{400, false},
		{404, false},
		{500, false},
	}
	for _, tt := range tests {
		if got := isRedirect(tt.code); got != tt.want {
			t.Errorf("isRedirect(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

// TestRangeStartMatchesOffset covers Content-Range header parsing.
func TestRangeStartMatchesOffset(t *testing.T) {
	tests := []struct {
		header string
		offset int64
		want   bool
	}{
		{"bytes 100-200/500", 100, true},
		{"bytes 100-200/500", 50, false},
		{"bytes 0-99/100", 0, true},
		{"", 100, false},
		{"invalid", 100, false},
		{"bytes abc-200/500", 100, false},
		{"bytes 100-200/*", 100, true},
	}
	for _, tt := range tests {
		if got := rangeStartMatchesOffset(tt.header, tt.offset); got != tt.want {
			t.Errorf("rangeStartMatchesOffset(%q, %d) = %v, want %v", tt.header, tt.offset, got, tt.want)
		}
	}
}
