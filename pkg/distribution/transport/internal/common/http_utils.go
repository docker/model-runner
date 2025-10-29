// Package common provides shared utilities for HTTP transport implementations.
package common

import (
	"net/http"
	"strconv"
	"strings"
)

// SupportsRange determines whether an HTTP response indicates support for range requests.
func SupportsRange(h http.Header) bool {
	ar := strings.ToLower(h.Get("Accept-Ranges"))
	for _, part := range strings.Split(ar, ",") {
		if strings.TrimSpace(part) == "bytes" {
			return true
		}
	}
	return false
}

// ScrubConditionalHeaders removes conditional headers we do not want to forward
// on range requests, because they can alter semantics or conflict with If-Range logic.
func ScrubConditionalHeaders(h http.Header) {
	h.Del("If-None-Match")
	h.Del("If-Modified-Since")
	h.Del("If-Match")
	h.Del("If-Unmodified-Since")
	// Range/If-Range headers are set explicitly by the caller.
}

// IsWeakETag reports whether the ETag is a weak validator (W/"...") which must
// not be used with If-Range per RFC 7232 §2.1.
func IsWeakETag(etag string) bool {
	etag = strings.TrimSpace(etag)
	return strings.HasPrefix(etag, "W/") || strings.HasPrefix(etag, "w/")
}

// ParseSingleRange parses a single "Range: bytes=start-end" header.
// It returns (start, end, ok). When end is omitted, end == -1.
//
// Notes:
//   - Only absolute-start forms are supported (no suffix ranges "-N").
//   - Multi-range specifications (comma separated) return ok == false.
func ParseSingleRange(h string) (int64, int64, bool) {
	if h == "" {
		return 0, -1, false
	}
	h = strings.TrimSpace(h)
	if !strings.HasPrefix(strings.ToLower(h), "bytes=") {
		return 0, -1, false
	}
	spec := strings.TrimSpace(h[len("bytes="):])
	if strings.Contains(spec, ",") {
		return 0, -1, false
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, -1, false
	}
	if parts[0] == "" {
		// Suffix form is not supported here.
		return 0, -1, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 {
		return 0, -1, false
	}
	end := int64(-1)
	if strings.TrimSpace(parts[1]) != "" {
		e, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || e < start {
			return 0, -1, false
		}
		end = e
	}
	return start, end, true
}

// ParseContentRange parses "Content-Range: bytes start-end/total". It
// returns (start, end, total, ok). When total is unknown, total == -1.
func ParseContentRange(h string) (int64, int64, int64, bool) {
	if h == "" {
		return 0, -1, -1, false
	}
	h = strings.ToLower(strings.TrimSpace(h))
	if !strings.HasPrefix(h, "bytes ") {
		return 0, -1, -1, false
	}
	body := strings.TrimSpace(h[len("bytes "):])
	seTotal := strings.SplitN(body, "/", 2)
	if len(seTotal) != 2 {
		return 0, -1, -1, false
	}
	se := strings.SplitN(strings.TrimSpace(seTotal[0]), "-", 2)
	if len(se) != 2 {
		return 0, -1, -1, false
	}
	start, err1 := strconv.ParseInt(strings.TrimSpace(se[0]), 10, 64)
	end, err2 := strconv.ParseInt(strings.TrimSpace(se[1]), 10, 64)
	totalStr := strings.TrimSpace(seTotal[1])
	var total int64 = -1
	var err3 error
	if totalStr != "*" {
		total, err3 = strconv.ParseInt(totalStr, 10, 64)
	}
	if err1 != nil || err2 != nil || (err3 != nil && totalStr != "*") {
		return 0, -1, -1, false
	}
	return start, end, total, true
}
