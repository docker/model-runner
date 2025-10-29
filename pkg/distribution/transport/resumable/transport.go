// Package resumable provides an http.RoundTripper that transparently resumes
// interrupted GET responses from servers that support byte ranges.
//
// ───────────────────────────── How it works ─────────────────────────────
//   - For GET responses with status 200 or 206 and "Accept-Ranges: bytes",
//     the transport replaces resp.Body with a resumable reader.
//   - If a mid-stream read fails (e.g., connection cut), it issues a follow-up
//     request with a "Range" header to continue from the last delivered byte.
//     It uses ETag (strong only) or Last-Modified via If-Range for safety.
//   - If the server doesn’t support ranges (or for non-GET), it passes
//     through the response unmodified.
//
// ───────────────────────────── Notes & caveats ───────────────────────────
//   - Only single byte ranges are supported when the original request already
//     includes Range (multi-range requests are passed through without resuming).
//   - Auto-decompression must not be active, or offsets won’t line up. If the
//     initial response was transparently decompressed (resp.Uncompressed == true)
//     or Content-Encoding was set, resumption is disabled for that response.
//   - Cookies added by an http.Client Jar after the initial response aren’t
//     automatically applied to follow-up range requests (since they bypass
//     http.Client). Existing request headers (incl. Cookie, Authorization, etc.)
//     are preserved, but Set-Cookie from the initial response won't be consulted.
//   - Some servers don’t advertise Accept-Ranges but still support Range.
//     This implementation requires explicit "Accept-Ranges: bytes" for safety.
package resumable

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/distribution/transport/internal/common"
)

// Option configures a ResumableTransport.
type Option func(*ResumableTransport)

// WithMaxRetries sets the maximum number of resume attempts after an error.
// Default: 3.
func WithMaxRetries(n int) Option {
	return func(rt *ResumableTransport) { rt.maxRetries = n }
}

// BackoffFunc computes the sleep duration for a given retry attempt (0-based).
type BackoffFunc func(attempt int) time.Duration

// WithBackoff sets the backoff strategy for resume attempts.
// Default: jittered exponential starting at 200ms, capped at 5s.
func WithBackoff(f BackoffFunc) Option {
	return func(rt *ResumableTransport) { rt.backoff = f }
}

// ResumableTransport wraps another http.RoundTripper and transparently retries
// mid-stream failures for GET requests against servers that support range requests.
type ResumableTransport struct {
	// base is the underlying RoundTripper actually used to send requests.
	base http.RoundTripper
	// maxRetries is the maximum number of resume attempts that will be made
	// after a read error before giving up.
	maxRetries int
	// backoff computes how long to wait before each retry attempt.
	// Called with the total number of attempts made so far (0-based).
	backoff BackoffFunc
}

// New returns a ResumableTransport wrapping base. If base is nil,
// http.DefaultTransport is used. Options configure retries/backoff.
func New(base http.RoundTripper, opts ...Option) *ResumableTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := &ResumableTransport{
		base:       base,
		maxRetries: 3,
		backoff: func(i int) time.Duration {
			// 200ms * 2^i with ±20% jitter, capped at 5s
			base := 200 * time.Millisecond
			d := time.Duration(float64(base) * math.Pow(2, float64(i)))
			if d > 5*time.Second {
				d = 5 * time.Second
			}
			j := 0.2 + rand.Float64()*0.4 // [0.2,0.6)
			return time.Duration(float64(d) * j)
		},
	}
	for _, o := range opts {
		o(rt)
	}
	return rt
}

// RoundTrip implements http.RoundTripper. It wraps GET requests that return
// 200/206 responses with "Accept-Ranges: bytes" support in a resumable body.
func (rt *ResumableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Always use the base transport to perform the initial request.
	resp, err := rt.base.RoundTrip(req)
	if resp == nil || err != nil {
		return resp, err
	}

	// If the request doesn't meet our resumability criteria, then return it
	// directly.
	if !isResumable(req, resp) {
		return resp, nil
	}

	// Create a resumable body to perform retries if needed.
	rb := newResumableBody(req, resp, rt)
	resp.Body = rb
	if n, ok := rb.plannedLength(); ok {
		resp.ContentLength = n
	} else {
		resp.ContentLength = -1
	}
	return resp, nil
}

// isResumable checks if the pair (request, response) is eligible for resume.
func isResumable(req *http.Request, resp *http.Response) bool {
	if req.Method != http.MethodGet {
		return false
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	if !common.SupportsRange(resp.Header) {
		return false
	}
	// Disallow when the response was auto-decompressed or has a Content-Encoding.
	if resp.Uncompressed || resp.Header.Get("Content-Encoding") != "" {
		return false
	}
	// If the original request specified a Range, only support single-range.
	if r := req.Header.Get("Range"); strings.TrimSpace(r) != "" {
		if _, _, ok := common.ParseSingleRange(r); !ok {
			return false
		}
	}
	return true
}

// resumableBody wraps a Response.Body to add transparent resume support.
// It keeps track of how many bytes have been delivered and re-issues
// Range requests starting from that offset when a read fails.
type resumableBody struct {
	// mu guards access to all fields below.
	mu sync.Mutex
	// ctx is the request context; canceled if caller cancels.
	ctx context.Context
	// tr is the owning ResumableTransport (for retry/backoff settings).
	tr *ResumableTransport
	// base is the underlying RoundTripper to use for follow-up Range requests.
	base http.RoundTripper
	// origReq is the original *http.Request; used as a template for retries.
	origReq *http.Request
	// current is the most recent *http.Response from which we are reading.
	current *http.Response
	// rc is the active body we are currently reading from.
	rc io.ReadCloser
	// bytesRead is how many bytes we have successfully delivered to the caller
	// (relative to initialStart).
	bytesRead int64
	// initialStart is the starting offset of the stream on the wire, usually 0.
	initialStart int64
	// initialEnd is the inclusive end offset (if known from Range header).
	initialEnd *int64
	// totalSize is the total known length of the resource, if available.
	totalSize *int64
	// etag is the validator used for If-Range in resumed requests (preferred).
	etag string
	// lastModified is a fallback validator for If-Range if no ETag is present.
	lastModified string
	// retriesUsed counts how many resume attempts we have made so far.
	retriesUsed int
	// originalRangeSpec is the Range header sent on the initial request.
	originalRangeSpec string
	// done marks that we’ve finished delivering all bytes (EOF).
	done bool
}

// newResumableBody constructs a resumableBody from the initial response.
func newResumableBody(req *http.Request, resp *http.Response, tr *ResumableTransport) *resumableBody {
	rb := &resumableBody{
		ctx:               req.Context(),
		tr:                tr,
		base:              tr.base,
		origReq:           req,
		current:           resp,
		rc:                resp.Body,
		originalRangeSpec: req.Header.Get("Range"),
	}

	// Extract starting offsets from request Range if present (single-range only).
	if start, end, ok := common.ParseSingleRange(rb.originalRangeSpec); ok {
		rb.initialStart = start
		if end >= 0 {
			rb.initialEnd = &end
		}
	}

	// Refine offsets from Content-Range header if response was 206.
	if resp.StatusCode == http.StatusPartialContent {
		if s, e, total, ok := common.ParseContentRange(resp.Header.Get("Content-Range")); ok {
			rb.initialStart = s
			if e >= 0 {
				rb.initialEnd = &e
			}
			if total >= 0 {
				rb.totalSize = &total
			}
		}
	} else if resp.StatusCode == http.StatusOK {
		// For 200 OK, the server is sending a full stream starting at 0
		// regardless of any Range header on the request.
		rb.initialStart = 0
		rb.initialEnd = nil
		if resp.ContentLength >= 0 {
			total := int64(resp.ContentLength)
			rb.totalSize = &total
		}
	}

	// Capture validators for If-Range to ensure consistency across resumes.
	if et := resp.Header.Get("ETag"); et != "" && !common.IsWeakETag(et) {
		rb.etag = et
	} else if lm := resp.Header.Get("Last-Modified"); lm != "" {
		rb.lastModified = lm
	}
	return rb
}

// Read delivers bytes to the caller. If an error occurs mid-stream, it will
// transparently try to resume by issuing a new Range request. When the total
// length is unknown (e.g., 200 OK without Content-Length), completeness cannot
// be verified precisely; in such cases EOF is treated as the natural end.
func (rb *resumableBody) Read(p []byte) (int, error) {
	for {
		// Snapshot state without holding the lock across I/O.
		rb.mu.Lock()
		if rb.done {
			rb.mu.Unlock()
			return 0, io.EOF
		}
		rc := rb.rc
		planned, plannedOK := rb.plannedLength()
		already := rb.bytesRead
		rb.mu.Unlock()

		if rc == nil {
			if err := rb.resume(already); err != nil {
				return 0, err
			}
			continue
		}

		n, err := rc.Read(p)

		rb.mu.Lock()
		rb.bytesRead += int64(n)

		switch {
		case err == nil:
			rb.mu.Unlock()
			return n, nil
		case errors.Is(err, io.EOF):
			// If planned length is known and we are short, resume.
			if plannedOK && already+int64(n) < planned {
				_ = rb.rc.Close()
				rb.rc = nil
				if rb.retriesUsed >= rb.tr.maxRetries {
					rb.mu.Unlock()
					return n, io.ErrUnexpectedEOF
				}
				// Return bytes now; resume on next call.
				if n > 0 {
					rb.mu.Unlock()
					return n, nil
				}
				// Resume outside lock.
				nextOffset := rb.bytesRead
				rb.mu.Unlock()
				if rerr := rb.resume(nextOffset); rerr != nil {
					return 0, rerr
				}
				continue
			}
			// Completed.
			rb.done = true
			rb.mu.Unlock()
			return n, io.EOF
		default:
			// Underlying read failed mid-stream. Try to resume.
			_ = rb.rc.Close()
			rb.rc = nil

			if n > 0 {
				rb.mu.Unlock()
				// Surface bytes already read; the caller will call Read again.
				return n, nil
			}
			if rb.retriesUsed >= rb.tr.maxRetries {
				rb.mu.Unlock()
				return 0, err
			}
			off := rb.bytesRead
			rb.mu.Unlock()
			if rerr := rb.resume(off); rerr != nil {
				return 0, rerr
			}
			continue
		}
	}
}

// Close closes the current response body if present.
func (rb *resumableBody) Close() error {
	rb.mu.Lock()
	rc := rb.rc
	rb.rc = nil
	rb.done = true
	rb.mu.Unlock()
	if rc != nil {
		return rc.Close()
	}
	return nil
}

// plannedLength returns the exact number of bytes this resumableBody intends to produce, if knowable.
func (rb *resumableBody) plannedLength() (int64, bool) {
	if rb.initialEnd != nil {
		return *rb.initialEnd - rb.initialStart + 1, true
	}
	if rb.current.StatusCode == http.StatusOK && rb.totalSize != nil {
		// 200 OK with known total size at start-of-stream.
		return *rb.totalSize, true
	}
	return 0, false
}

// resume attempts to resume the response stream at the given absolute offset
// (relative to the very first byte on the wire). The method will make up to the
// remaining retry budget attempts. On success it swaps rb.rc with a fresh body.
func (rb *resumableBody) resume(absoluteOffset int64) error {
	remaining := rb.tr.maxRetries - rb.retriesUsed
	for attempt := 0; attempt < remaining; attempt++ {
		if err := rb.ctx.Err(); err != nil {
			return err
		}

		// For safety, do not attempt an unvalidated resume when neither a
		// strong ETag nor Last-Modified validator is available.
		if rb.etag == "" && rb.lastModified == "" {
			return fmt.Errorf("resumable: cannot resume without validator")
		}

		start := rb.initialStart + absoluteOffset
		rangeVal := buildRangeHeader(start, rb.initialEnd)
		req := rb.cloneBaseRequest(rangeVal)

		// Backoff for subsequent attempts.
		if attempt > 0 || rb.retriesUsed > 0 {
			if err := waitBackoff(rb.ctx, rb.tr.backoff, rb.retriesUsed+attempt); err != nil {
				return err
			}
		}

		resp, err := rb.base.RoundTrip(req)
		if err != nil {
			continue // try again within budget
		}

		switch resp.StatusCode {
		case http.StatusPartialContent:
			// Validate server honored our starting offset precisely.
			s, e, _, ok := common.ParseContentRange(resp.Header.Get("Content-Range"))
			if !ok || s != start {
				_ = resp.Body.Close()
				continue // try again; mismatched range
			}
			// If we requested a closed range and the end does not match, do
			// not accept this response.
			if rb.initialEnd != nil && e >= 0 && e != *rb.initialEnd {
				_ = resp.Body.Close()
				continue
			}
			// Install the new response under lock.
			rb.mu.Lock()
			rb.installResponseLocked(resp)
			rb.retriesUsed++
			rb.mu.Unlock()
			return nil

		case http.StatusOK:
			// If we requested a range but got a full response, it likely means the
			// validator failed (resource changed) or the server ignored Range.
			_ = resp.Body.Close()
			return fmt.Errorf("resumable: server returned 200 to a range request; resource may have changed")

		case http.StatusMultipleChoices, http.StatusMovedPermanently, http.StatusFound,
			http.StatusSeeOther, http.StatusNotModified, http.StatusUseProxy,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			_ = resp.Body.Close()
			return fmt.Errorf("resumable: resume received redirect status %d", resp.StatusCode)

		case http.StatusRequestedRangeNotSatisfiable:
			// If we've already read to/ past the expected end, we are actually done.
			if rb.rangeIsComplete(absoluteOffset) {
				rb.done = true
				_ = resp.Body.Close()
				return io.EOF
			}
			_ = resp.Body.Close()

		default:
			_ = resp.Body.Close()
		}
	}
	return fmt.Errorf("resumable: exceeded retry budget after %d attempts", rb.tr.maxRetries)
}

// installResponseLocked installs resp as the current response and updates
// validators and size info. Caller must hold rb.mu.
func (rb *resumableBody) installResponseLocked(resp *http.Response) {
	if rb.rc != nil && rb.rc != resp.Body {
		_ = rb.rc.Close()
	}
	rb.current = resp
	rb.rc = resp.Body

	// Persist validators from the server if they are strong.
	if et := resp.Header.Get("ETag"); et != "" && !common.IsWeakETag(et) {
		rb.etag = et
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		rb.lastModified = lm
	}

	// Merge any updated size info from the Content-Range.
	if s, e, total, ok := common.ParseContentRange(resp.Header.Get("Content-Range")); ok {
		_ = s // start validated by caller
		if e >= 0 {
			rb.initialEnd = &e
		}
		if total >= 0 {
			rb.totalSize = &total
		}
	}
}

// cloneBaseRequest builds a new GET request with the same headers as the original,
// except with a different Range and If-Range validator. To avoid mismatched
// encodings, we also force identity encoding.
func (rb *resumableBody) cloneBaseRequest(rangeVal string) *http.Request {
	req := rb.origReq.Clone(rb.ctx)
	req.Body = nil
	req.ContentLength = 0
	req.Header = rb.origReq.Header.Clone()

	// Ensure we control the Range validator set.
	req.Header.Set("Range", rangeVal)
	// Remove conditional headers that could conflict with If-Range semantics.
	common.ScrubConditionalHeaders(req.Header)

	if rb.etag != "" {
		req.Header.Set("If-Range", rb.etag)
	} else if rb.lastModified != "" {
		req.Header.Set("If-Range", rb.lastModified)
	}

	// Prevent transparent decompression on resumed requests.
	req.Header.Set("Accept-Encoding", "identity")
	return req
}

// buildRangeHeader constructs a "Range" header value for a given start and
// optional inclusive end.
func buildRangeHeader(start int64, end *int64) string {
	if end == nil {
		return fmt.Sprintf("bytes=%d-", start)
	}
	return fmt.Sprintf("bytes=%d-%d", start, *end)
}

// waitBackoff sleeps using the provided backoff function, unless the context
// is canceled.
func waitBackoff(ctx context.Context, bf BackoffFunc, attempt int) error {
	d := time.Duration(0)
	if bf != nil {
		d = bf(attempt)
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// rangeIsComplete returns true if the bytes we have delivered already meet the
// expected end of the range / resource, so a 416 implies we are done.
func (rb *resumableBody) rangeIsComplete(absoluteOffset int64) bool {
	if rb.totalSize != nil {
		// If we know total size, we are complete if start+offset >= total.
		if rb.initialStart+absoluteOffset >= *rb.totalSize {
			return true
		}
	}
	if rb.initialEnd != nil {
		// initialEnd is inclusive.
		if rb.initialStart+absoluteOffset >= *rb.initialEnd+1 {
			return true
		}
	}
	return false
}
