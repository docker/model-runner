// Package parallel provides an http.RoundTripper that transparently
// parallelizes GET requests using concurrent byte-range requests for better
// throughput.
//
// ───────────────────────────── How it works ─────────────────────────────
//   - For non-GET requests, the transport passes them through unmodified to
//     the underlying transport.
//   - For GET requests, it first performs a HEAD request to check if the
//     server supports byte ranges and to determine the total response size.
//   - If the HEAD request indicates range support and known size, the
//     transport generates multiple concurrent GET requests with specific
//     byte-range headers.
//   - Subranges are written to temporary files and stitched together in a
//     custom Response.Body that's transparent to the caller.
//   - Per-host and per-request concurrency limits are enforced using
//     semaphores.
//
// ───────────────────────────── Notes & caveats ───────────────────────────
//   - Only works with servers that support "Accept-Ranges: bytes" and
//     provide Content-Length or Content-Range headers with total size
//     information.
//   - Content-Encoding (compression) is not compatible with byte ranges,
//     so compressed responses fall back to single-threaded behavior.
//   - Temporary files are created for each subrange and cleaned up
//     automatically.
//   - The transport respects per-host concurrency limits to avoid
//     overwhelming servers.
package parallel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/model-runner/pkg/distribution/transport/internal/bufferfile"
	"github.com/docker/model-runner/pkg/distribution/transport/internal/common"
)

// Option configures a ParallelTransport.
type Option func(*ParallelTransport)

// WithMaxConcurrentPerHost sets the maximum concurrent requests per
// hostname. Default concurrency limits are applied if not specified.
func WithMaxConcurrentPerHost(limits map[string]uint) Option {
	return func(pt *ParallelTransport) {
		pt.maxConcurrentPerHost = make(map[string]uint, len(limits))
		for host, limit := range limits {
			pt.maxConcurrentPerHost[host] = limit
		}
	}
}

// WithMaxConcurrentPerRequest sets the maximum concurrent subrange
// requests for a single request. Default: 4.
func WithMaxConcurrentPerRequest(n uint) Option {
	return func(pt *ParallelTransport) { pt.maxConcurrentPerRequest = n }
}

// WithMinChunkSize sets the minimum size in bytes for each subrange chunk.
// Requests smaller than this will not be parallelized. Default: 1MB.
func WithMinChunkSize(size int64) Option {
	return func(pt *ParallelTransport) { pt.minChunkSize = size }
}

// WithTempDir sets the directory for temporary files. If empty,
// os.TempDir() is used.
func WithTempDir(dir string) Option {
	return func(pt *ParallelTransport) { pt.tempDir = dir }
}

// ParallelTransport wraps another http.RoundTripper and parallelizes GET
// requests using concurrent byte-range requests when possible.
type ParallelTransport struct {
	// base is the underlying RoundTripper actually used to send requests.
	base http.RoundTripper
	// maxConcurrentPerHost maps canonicalized hostname to maximum
	// concurrent requests. A value of 0 means unlimited. The "" entry is
	// the default for unspecified hosts.
	maxConcurrentPerHost map[string]uint
	// maxConcurrentPerRequest is the maximum number of concurrent
	// subrange requests for a single request.
	maxConcurrentPerRequest uint
	// minChunkSize is the minimum size in bytes for parallelization to be
	// worthwhile.
	minChunkSize int64
	// tempDir is the directory for temporary files.
	tempDir string
	// semaphores tracks per-host concurrency limits.
	semaphores map[string]*semaphore
	// semMu protects the semaphores map.
	semMu sync.RWMutex
}

// New returns a ParallelTransport wrapping base. If base is nil,
// http.DefaultTransport is used. Options configure parallelization behavior.
func New(base http.RoundTripper, opts ...Option) *ParallelTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	pt := &ParallelTransport{
		base:                    base,
		maxConcurrentPerHost:    map[string]uint{"": 4}, // default 4 per host.
		maxConcurrentPerRequest: 4,
		minChunkSize:            1024 * 1024, // 1MB.
		tempDir:                 os.TempDir(),
		semaphores:              make(map[string]*semaphore),
	}
	for _, o := range opts {
		o(pt)
	}
	return pt
}

// RoundTrip implements http.RoundTripper. It parallelizes GET requests
// when possible, otherwise passes requests through to the underlying
// transport.
func (pt *ParallelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Non-GET requests pass through unmodified.
	if req.Method != http.MethodGet {
		return pt.base.RoundTrip(req)
	}
	// Respect caller-provided Range requests. We do not parallelize when the
	// request already specifies a byte range, to preserve exact semantics.
	if strings.TrimSpace(req.Header.Get("Range")) != "" {
		return pt.base.RoundTrip(req)
	}

	// Check if parallelization is possible and worthwhile.
	canParallelize, pInfo, err := pt.checkParallelizable(req)
	if err != nil {
		return nil, err
	}
	if !canParallelize ||
		pInfo.totalSize < pt.minChunkSize*int64(pt.maxConcurrentPerRequest) {
		// Fall back to single request.
		return pt.base.RoundTrip(req)
	}

	// Perform parallel download.
	return pt.parallelDownload(req, pInfo)
}

// parallelInfo holds information needed for parallel downloads.
type parallelInfo struct {
	// totalSize is the total size of the resource in bytes.
	totalSize int64
	// etag is the strong ETag validator from the HEAD response, used for
	// If-Range.
	etag string
	// lastModified is the Last-Modified header value, used as fallback
	// validator for If-Range.
	lastModified string
	// header is a clone of the server headers (from HEAD) used to seed the
	// final response headers without an extra GET probe.
	header http.Header
	// proto/protoMajor/protoMinor reflect the server protocol from the HEAD
	// response for constructing the final response.
	proto      string
	protoMajor int
	protoMinor int
}

// checkParallelizable performs a HEAD request to determine if the resource
// supports byte ranges and returns the parallel info if available.
func (pt *ParallelTransport) checkParallelizable(req *http.Request) (bool, *parallelInfo, error) {
	// Create HEAD request.
	headReq := req.Clone(req.Context())
	headReq.Method = http.MethodHead
	headReq.Body = nil
	headReq.ContentLength = 0
	// Clone and sanitize headers to avoid conditional responses and implicit
	// compression that could skew metadata.
	headReq.Header = req.Header.Clone()
	common.ScrubConditionalHeaders(headReq.Header)
	headReq.Header.Set("Accept-Encoding", "identity")

	// Perform HEAD request.
	headResp, err := pt.base.RoundTrip(headReq)
	if err != nil {
		return false, nil, err
	}
	defer headResp.Body.Close()

	// Only proceed on 200 OK or 206 Partial Content. Anything else (e.g.,
	// 304 Not Modified due to missed scrub, redirects, etc.) is treated as
	// non-parallelizable for safety.
	if headResp.StatusCode != http.StatusOK &&
		headResp.StatusCode != http.StatusPartialContent {
		return false, nil, nil
	}

	// Check if range requests are supported.
	if !common.SupportsRange(headResp.Header) {
		return false, nil, nil
	}

	// Check for compression which would interfere with byte ranges.
	if headResp.Header.Get("Content-Encoding") != "" {
		return false, nil, nil
	}

	// Get total content length.
	totalSize := headResp.ContentLength
	if totalSize <= 0 {
		// Try to parse from Content-Range if present (206 response).
		if headResp.StatusCode == http.StatusPartialContent {
			if _, _, total, ok := common.ParseContentRange(
				headResp.Header.Get("Content-Range")); ok && total > 0 {
				totalSize = total
			} else {
				return false, nil, nil
			}
		} else {
			return false, nil, nil
		}
	}

	if totalSize <= 0 {
		return false, nil, nil
	}

	// Capture validators for If-Range to ensure consistency across parallel
	// requests.
	info := &parallelInfo{
		totalSize:  totalSize,
		header:     headResp.Header.Clone(),
		proto:      headResp.Proto,
		protoMajor: headResp.ProtoMajor,
		protoMinor: headResp.ProtoMinor,
	}

	if et := headResp.Header.Get("ETag"); et != "" && !common.IsWeakETag(et) {
		info.etag = et
	} else if lm := headResp.Header.Get("Last-Modified"); lm != "" {
		info.lastModified = lm
	}

	return true, info, nil
}

// parallelDownload performs a parallel download by splitting the request
// into multiple concurrent byte-range requests.
func (pt *ParallelTransport) parallelDownload(req *http.Request, pInfo *parallelInfo) (*http.Response, error) {
	totalSize := pInfo.totalSize

	// Calculate chunk size and number of chunks.
	numChunks := int(pt.maxConcurrentPerRequest)
	if totalSize < int64(numChunks)*pt.minChunkSize {
		numChunks = int(totalSize / pt.minChunkSize)
		if numChunks < 1 {
			numChunks = 1
		}
	}

	chunkSize := totalSize / int64(numChunks)
	remainder := totalSize % int64(numChunks)

	// Get or create semaphore for this host.
	sem := pt.getSemaphore(req.URL.Host)

	// Create chunks and temporary files.
	chunks := make([]*chunk, numChunks)
	var start int64
	for i := 0; i < numChunks; i++ {
		size := chunkSize
		if i == numChunks-1 {
			size += remainder // Last chunk gets the remainder.
		}
		end := start + size - 1

		fifo, err := bufferfile.NewFIFOInDir(pt.tempDir)
		if err != nil {
			// Clean up any created FIFOs.
			for j := 0; j < i; j++ {
				chunks[j].cleanup()
			}
			return nil, fmt.Errorf("parallel: failed to create FIFO: %w", err)
		}

		chunk := &chunk{
			start: start,
			end:   end,
			fifo:  fifo,
			state: chunkNotStarted,
		}
		chunks[i] = chunk
		start = end + 1
	}

	// Start downloading chunks concurrently (don't wait for completion).
	for i, ch := range chunks {
		go func(i int, ch *chunk) {
			ch.setSimpleState(chunkDownloading, nil)
			if err := pt.downloadChunk(req, ch, sem, pInfo); err != nil {
				ch.setSimpleState(chunkFailed, fmt.Errorf("chunk %d: %w", i, err))
				ch.fifo.Close() // Close FIFO on error to interrupt readers.
			} else {
				ch.setSimpleState(chunkCompleted, nil)
				// Close write side to signal no more writes (EOF when all data
				// read).
				ch.fifo.CloseWrite()
			}
		}(i, ch)
	}

	// Create stitched response.
	body := &stitchedBody{
		chunks:    chunks,
		totalSize: totalSize,
		ctx:       req.Context(),
	}

	// Create response using the header response as template.
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         pInfo.proto,
		ProtoMajor:    pInfo.protoMajor,
		ProtoMinor:    pInfo.protoMinor,
		Header:        pInfo.header.Clone(),
		Body:          body,
		ContentLength: totalSize,
		Request:       req,
	}

	// Override headers that we control.
	resp.Header.Set("Content-Length", strconv.FormatInt(totalSize, 10))
	resp.Header.Del("Content-Range") // Remove any partial content headers.

	return resp, nil
}

// downloadChunk downloads a single chunk using a byte-range request.
func (pt *ParallelTransport) downloadChunk(origReq *http.Request, chunk *chunk, sem *semaphore, pInfo *parallelInfo) error {
	// Acquire semaphore.
	if err := sem.acquire(origReq.Context()); err != nil {
		return err
	}
	defer sem.release()

	// Create range request.
	rangeReq := origReq.Clone(origReq.Context())
	rangeReq.Header = origReq.Header.Clone()
	rangeReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.start, chunk.end))

	// Prevent compression which would interfere with byte ranges.
	rangeReq.Header.Set("Accept-Encoding", "identity")

	// Add If-Range header for consistency validation.
	if pInfo.etag != "" {
		rangeReq.Header.Set("If-Range", pInfo.etag)
	} else if pInfo.lastModified != "" {
		rangeReq.Header.Set("If-Range", pInfo.lastModified)
	}

	// Remove conditional headers that could conflict with If-Range.
	common.ScrubConditionalHeaders(rangeReq.Header)

	// Perform request.
	resp, err := pt.base.RoundTrip(rangeReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for If-Range validation failure (server returns 200 instead of 206).
	if resp.StatusCode == http.StatusOK {
		return fmt.Errorf(
			"server returned 200 to range request, resource may have changed (If-Range validation failed)")
	}

	// Verify we got a partial content response.
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf(
			"expected 206 Partial Content, got %d", resp.StatusCode)
	}

	// Verify the range matches what we requested.
	if start, end, _, ok := common.ParseContentRange(resp.Header.Get("Content-Range")); ok {
		if start != chunk.start || end != chunk.end {
			return fmt.Errorf(
				"server returned range %d-%d, requested %d-%d",
				start, end, chunk.start, chunk.end)
		}
	}

	// Copy response body to FIFO and verify full chunk length is received.
	buf := make([]byte, 32*1024) // 32KB buffer.
	var copied int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			// Write to FIFO
			if _, writeErr := chunk.fifo.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf(
					"failed to write chunk data: %w", writeErr)
			}
			copied += int64(n)
		}

		if err == io.EOF {
			// Validate that we received the complete range we requested.
			expected := (chunk.end - chunk.start + 1)
			if copied != expected {
				return fmt.Errorf(
					"short read for chunk: got %d, want %d", copied, expected)
			}
			break
		}
		if err != nil {
			return fmt.Errorf(
				"failed to read chunk data: %w", err)
		}
	}

	return nil
}

// getSemaphore returns the semaphore for the given host, creating it if needed.
func (pt *ParallelTransport) getSemaphore(host string) *semaphore {
	canonicalHost := canonicalizeHost(host)

	pt.semMu.RLock()
	if sem, exists := pt.semaphores[canonicalHost]; exists {
		pt.semMu.RUnlock()
		return sem
	}
	pt.semMu.RUnlock()

	pt.semMu.Lock()
	defer pt.semMu.Unlock()

	// Double-check after acquiring write lock.
	if sem, exists := pt.semaphores[canonicalHost]; exists {
		return sem
	}

	// Determine limit for this host.
	limit := pt.maxConcurrentPerHost[canonicalHost]
	if limit == 0 {
		// Check default.
		if defaultLimit, exists := pt.maxConcurrentPerHost[""]; exists {
			limit = defaultLimit
		}
	}

	sem := newSemaphore(int(limit))
	pt.semaphores[canonicalHost] = sem
	return sem
}

// canonicalizeHost returns a canonical form of the hostname for semaphore lookup.
func canonicalizeHost(host string) string {
	// Remove port if present.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

// chunkState represents the current state of a chunk download.
type chunkState int

const (
	chunkNotStarted chunkState = iota
	chunkDownloading
	chunkCompleted
	chunkFailed
)

// chunk represents a byte range chunk being downloaded to a temporary file.
type chunk struct {
	// start is the inclusive starting byte offset for this chunk.
	start int64
	// end is the inclusive ending byte offset for this chunk.
	end int64
	// fifo is the FIFO buffer where this chunk's data is stored.
	fifo *bufferfile.FIFO
	// state tracks the current download state of this chunk.
	state chunkState
	// err holds any error that occurred during download.
	err error
	// mu protects state and err fields.
	mu sync.Mutex
}

// close closes the FIFO handle.
func (c *chunk) close() error {
	if c.fifo == nil {
		return nil
	}
	return c.fifo.Close()
}

// cleanup closes and removes the FIFO.
func (c *chunk) cleanup() {
	if c.fifo != nil {
		// Only close the FIFO. Do not nil the pointer to avoid races with
		// in-flight writer goroutines checking or using this handle.
		c.fifo.Close()
	}
}

// setSimpleState updates the chunk state. No condition signaling needed since FIFO handles coordination.
func (c *chunk) setSimpleState(state chunkState, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = state
	c.err = err
}

// readAvailable reads up to len(p) bytes from the chunk, blocking until data is available.
// Returns the number of bytes read and any error. Returns io.EOF when chunk is complete
// and all data has been read.
func (c *chunk) readAvailable(p []byte, ctx context.Context) (int, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// Check if chunk failed first
	c.mu.Lock()
	if c.state == chunkFailed && c.err != nil {
		err := c.err
		c.mu.Unlock()
		return 0, err
	}
	c.mu.Unlock()

	// Try to read from FIFO
	n, err := c.fifo.Read(p)

	// If we got data, return it
	if n > 0 {
		return n, nil
	}

	// If FIFO is closed or returned EOF, check chunk state
	if err == io.EOF {
		// If chunk is completed and FIFO EOF, we're truly done
		c.mu.Lock()
		if c.state == chunkCompleted {
			c.mu.Unlock()
			return 0, io.EOF
		}
		c.mu.Unlock()
		// If chunk not completed but FIFO EOF, there might be an error
		// Fall through to return the EOF
	}

	return n, err
}

// stitchedBody implements io.ReadCloser by reading from multiple chunk files in sequence.
type stitchedBody struct {
	// chunks is the ordered list of chunk files to read from.
	chunks []*chunk
	// totalSize is the expected total number of bytes across all chunks.
	totalSize int64
	// currentIdx is the index of the chunk currently being read from.
	currentIdx int
	// bytesRead is the total number of bytes delivered to callers so far.
	bytesRead int64
	// closed indicates whether Close() has been called.
	closed bool
	// ctx is the request context for cancellation.
	ctx context.Context
	// mu protects all fields from concurrent access.
	mu sync.Mutex
}

// Read reads data by stitching together chunks in order.
func (sb *stitchedBody) Read(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.closed {
		return 0, errors.New("stitchedBody: read from closed body")
	}

	if sb.currentIdx >= len(sb.chunks) {
		return 0, io.EOF
	}

	totalRead := 0
	for len(p) > 0 && sb.currentIdx < len(sb.chunks) {
		ch := sb.chunks[sb.currentIdx]

		// Unlock while reading from chunk (chunk handles its own locking)
		sb.mu.Unlock()

		// Read available data from current chunk
		n, err := ch.readAvailable(p, sb.ctx)

		// Re-lock to update state
		sb.mu.Lock()

		if sb.closed {
			return totalRead, errors.New("stitchedBody: read from closed body")
		}

		if n > 0 {
			totalRead += n
			sb.bytesRead += int64(n)
			p = p[n:]
		}

		if err == io.EOF {
			// Current chunk is complete, move to next
			sb.currentIdx++
		} else if err != nil {
			return totalRead, fmt.Errorf("stitchedBody: chunk %d error: %w", sb.currentIdx, err)
		} else if n == 0 {
			// No error but no data read - this shouldn't happen with readAvailable
			// but handle it to avoid infinite loops
			return totalRead, fmt.Errorf("stitchedBody: chunk %d read 0 bytes without error or EOF", sb.currentIdx)
		}
	}

	if totalRead == 0 && sb.currentIdx >= len(sb.chunks) {
		return 0, io.EOF
	}

	return totalRead, nil
}

// Close closes all chunk files and cleans up temporary files.
func (sb *stitchedBody) Close() error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.closed {
		return nil
	}
	sb.closed = true

	var errs []error
	for _, ch := range sb.chunks {
		if err := ch.close(); err != nil {
			errs = append(errs, err)
		}
		ch.cleanup()
	}

	if len(errs) > 0 {
		return fmt.Errorf("stitchedBody: close errors: %v", errs)
	}
	return nil
}

// semaphore implements a counting semaphore for limiting concurrency.
type semaphore struct {
	// ch is the buffered channel used to limit concurrent operations.
	// If nil, no limits are enforced (unlimited concurrency).
	ch chan struct{}
}

// newSemaphore creates a new semaphore with the given capacity.
// If capacity is 0 or negative, the semaphore allows unlimited concurrency.
func newSemaphore(capacity int) *semaphore {
	if capacity <= 0 {
		// Unlimited semaphore - nil channel means no limits.
		return &semaphore{}
	}
	return &semaphore{
		ch: make(chan struct{}, capacity),
	}
}

// acquire acquires a semaphore slot, blocking until one is available or context is canceled.
func (s *semaphore) acquire(ctx context.Context) error {
	if s.ch == nil {
		// Unlimited semaphore - no need to acquire.
		return nil
	}
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release releases a semaphore slot.
func (s *semaphore) release() {
	if s.ch == nil {
		// Unlimited semaphore - no need to release.
		return
	}
	<-s.ch
}
