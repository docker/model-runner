package common

import (
	"net/http"
	"testing"
)

// TestParseSingleRange exercises valid and invalid single-range specs.
func TestParseSingleRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int64
		ok         bool
	}{
		{"", 0, -1, false},
		{"bytes=0-99", 0, 99, true},
		{"bytes=0-", 0, -1, true},
		{"bytes=5-5", 5, 5, true},
		{"BYTES=7-9", 7, 9, true},
		// End before start.
		{"bytes=10-5", 0, -1, false},
		// Suffix not supported.
		{"bytes=-100", 0, -1, false},
		{"items=0-10", 0, -1, false},
		// Multi-range unsupported.
		{"bytes=0-1,3-5", 0, -1, false},
	}
	for _, tc := range cases {
		start, end, ok := ParseSingleRange(tc.in)
		if start != tc.start || end != tc.end || ok != tc.ok {
			t.Errorf("ParseSingleRange(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.in, start, end, ok, tc.start, tc.end, tc.ok)
		}
	}
}

// TestParseContentRange exercises valid and invalid Content-Range headers.
func TestParseContentRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int64
		total      int64
		ok         bool
	}{
		{"", 0, -1, -1, false},
		{"bytes 0-99/200", 0, 99, 200, true},
		{"BYTES 1-1/2", 1, 1, 2, true},
		{"bytes 0-0/*", 0, 0, -1, true},
		{"items 0-1/2", 0, -1, -1, false},
		{"bytes 0-99/abc", 0, -1, -1, false},
		// Parser accepts; semantic check happens elsewhere.
		{"bytes 5-4/10", 5, 4, 10, true},
	}
	for _, tc := range cases {
		start, end, total, ok := ParseContentRange(tc.in)
		if start != tc.start || end != tc.end || total != tc.total || ok != tc.ok {
			t.Errorf("ParseContentRange(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)", tc.in, start, end, total, ok, tc.start, tc.end, tc.total, tc.ok)
		}
	}
}

// TestSupportsRange tests the Accept-Ranges header parsing.
func TestSupportsRange(t *testing.T) {
	cases := []struct {
		name     string
		header   http.Header
		expected bool
	}{
		{
			name:     "no header",
			header:   http.Header{},
			expected: false,
		},
		{
			name:     "bytes supported",
			header:   http.Header{"Accept-Ranges": []string{"bytes"}},
			expected: true,
		},
		{
			name:     "bytes with mixed case",
			header:   http.Header{"Accept-Ranges": []string{"BYTES"}},
			expected: true,
		},
		{
			name:     "bytes with other values",
			header:   http.Header{"Accept-Ranges": []string{"none, bytes"}},
			expected: true,
		},
		{
			name:     "none only",
			header:   http.Header{"Accept-Ranges": []string{"none"}},
			expected: false,
		},
		{
			name:     "other unit",
			header:   http.Header{"Accept-Ranges": []string{"items"}},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := SupportsRange(tc.header)
			if result != tc.expected {
				t.Errorf("SupportsRange() = %v, want %v", result, tc.expected)
			}
		})
	}
}

// TestIsWeakETag tests weak ETag detection.
func TestIsWeakETag(t *testing.T) {
	cases := []struct {
		name     string
		etag     string
		expected bool
	}{
		{
			name:     "strong etag",
			etag:     `"abc123"`,
			expected: false,
		},
		{
			name:     "weak etag uppercase W",
			etag:     `W/"abc123"`,
			expected: true,
		},
		{
			name:     "weak etag lowercase w",
			etag:     `w/"abc123"`,
			expected: true,
		},
		{
			name:     "empty",
			etag:     "",
			expected: false,
		},
		{
			name:     "with spaces",
			etag:     `  W/"abc123"  `,
			expected: true,
		},
		{
			name:     "malformed but starts with W",
			etag:     "W/malformed",
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsWeakETag(tc.etag)
			if result != tc.expected {
				t.Errorf("IsWeakETag(%q) = %v, want %v", tc.etag, result, tc.expected)
			}
		})
	}
}

// TestScrubConditionalHeaders tests conditional header removal.
func TestScrubConditionalHeaders(t *testing.T) {
	// Set up test headers with both conditional and non-conditional headers.
	headers := http.Header{
		"If-None-Match":       []string{`"etag1"`},
		"If-Modified-Since":   []string{"Wed, 21 Oct 2015 07:28:00 GMT"},
		"If-Match":            []string{`"etag2"`},
		"If-Unmodified-Since": []string{"Thu, 22 Oct 2015 07:28:00 GMT"},
		"Range":               []string{"bytes=0-99"},
		"If-Range":            []string{`"etag3"`},
		"Authorization":       []string{"Bearer token"},
	}

	// Scrub the conditional headers.
	ScrubConditionalHeaders(headers)

	// Verify conditional headers are removed.
	conditionalHeaders := []string{
		"If-None-Match",
		"If-Modified-Since",
		"If-Match",
		"If-Unmodified-Since",
	}
	for _, header := range conditionalHeaders {
		if headers.Get(header) != "" {
			t.Errorf("conditional header %s was not scrubbed", header)
		}
	}

	// Verify other headers are preserved.
	preservedHeaders := []string{"Range", "If-Range", "Authorization"}
	for _, header := range preservedHeaders {
		if headers.Get(header) == "" {
			t.Errorf("header %s was incorrectly removed", header)
		}
	}
}
