package parallel

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"

	testutil "github.com/docker/model-runner/pkg/distribution/transport/internal/testing"
)

// deterministicDataGenerator generates deterministic data based on position.
// This allows us to generate GB-sized data streams without storing them in
// memory.
type deterministicDataGenerator struct {
	position int64
	size     int64
}

// newDeterministicDataGenerator creates a new deterministic data generator
// with the specified size.
func newDeterministicDataGenerator(size int64) *deterministicDataGenerator {
	return &deterministicDataGenerator{
		position: 0,
		size:     size,
	}
}

// Read implements io.Reader for deterministicDataGenerator.
func (g *deterministicDataGenerator) Read(p []byte) (int, error) {
	if g.position >= g.size {
		return 0, io.EOF
	}

	// Calculate how much we can read.
	remaining := g.size - g.position
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Generate deterministic data based on position.
	for i := int64(0); i < toRead; i++ {
		pos := g.position + i
		// Use a simple but deterministic pattern: position mod 256.
		// XOR with some constants to make it more interesting.
		p[i] = byte((pos ^ (pos >> 8) ^ (pos >> 16)) % 256)
	}

	g.position += toRead
	return int(toRead), nil
}

// ReadAt implements io.ReaderAt for deterministicDataGenerator.
func (g *deterministicDataGenerator) ReadAt(p []byte, off int64) (int, error) {
	if off >= g.size {
		return 0, io.EOF
	}

	remaining := g.size - off
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	for i := int64(0); i < toRead; i++ {
		pos := off + i
		p[i] = byte((pos ^ (pos >> 8) ^ (pos >> 16)) % 256)
	}

	if toRead < int64(len(p)) {
		return int(toRead), io.EOF
	}

	return int(toRead), nil
}

// addLargeFileResource registers a deterministic large file with the fake
// transport. The resource shares behavior with the previous httptest server
// implementation, including range support and metadata headers.
func addLargeFileResource(ft *testutil.FakeTransport, url string, size int64) {
	ft.Add(url, &testutil.FakeResource{
		Data:          newDeterministicDataGenerator(size),
		Length:        size,
		SupportsRange: true,
		ETag:          fmt.Sprintf(`"test-file-%d"`, size),
		ContentType:   "application/octet-stream",
	})
}

// hashingReader wraps an io.Reader and computes SHA-256 while reading.
type hashingReader struct {
	reader    io.Reader
	hasher    hash.Hash
	bytesRead int64
}

// newHashingReader creates a new hashing reader that computes SHA-256
// hash while reading from the provided reader.
func newHashingReader(r io.Reader) *hashingReader {
	return &hashingReader{
		reader:    r,
		hasher:    sha256.New(),
		bytesRead: 0,
	}
}

// Read implements io.Reader for hashingReader.
func (hr *hashingReader) Read(p []byte) (int, error) {
	n, err := hr.reader.Read(p)
	if n > 0 {
		hr.hasher.Write(p[:n])
		hr.bytesRead += int64(n)
	}
	return n, err
}

// Sum returns the SHA-256 hash of all data read so far.
func (hr *hashingReader) Sum() []byte {
	return hr.hasher.Sum(nil)
}

// BytesRead returns the total number of bytes read.
func (hr *hashingReader) BytesRead() int64 {
	return hr.bytesRead
}

// computeExpectedHash computes the expected SHA-256 hash for a file of
// given size.
func computeExpectedHash(size int64) []byte {
	hasher := sha256.New()
	gen := newDeterministicDataGenerator(size)
	io.Copy(hasher, gen)
	return hasher.Sum(nil)
}

// getTestFileSize returns an appropriate file size for testing based on
// whether we're running under the race detector or other conditions.
// The returned size ensures parallel downloads will still occur (larger than
// typical minimum chunk sizes of 1-10MB).
func getTestFileSize(baseSize int64) int64 {
	// Allow environment override for custom testing.
	if sizeStr := os.Getenv("TEST_FILE_SIZE"); sizeStr != "" {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			return size
		}
	}

	// Check for race detector or coverage mode.
	if testing.CoverMode() != "" || raceEnabled {
		// Use ~200MB for "large" (1GB) and ~400MB for "very large" (4GB).
		// This is large enough to trigger parallel downloads with typical
		// chunk sizes of 4-8MB, but small enough to run quickly.
		if baseSize >= 4*1024*1024*1024 {
			return 400 * 1024 * 1024 // 400MB instead of 4GB.
		}
		return 200 * 1024 * 1024 // 200MB instead of 1GB.
	}

	return baseSize
}

// TestLargeFile_ParallelVsSequential tests parallel vs sequential
// download of a large file. The actual file size adapts based on whether
// the race detector is enabled (200MB in race mode, 1GB normally).
func TestLargeFile_ParallelVsSequential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Test with large file (1GB normally, 200MB in race/coverage mode).
	baseSize := int64(1024 * 1024 * 1024) // 1 GB base size.
	size := getTestFileSize(baseSize)

	if size != baseSize {
		t.Logf("Running with reduced file size: %d MB (race detector or coverage mode detected)",
			size/(1024*1024))
	}

	url := fmt.Sprintf("https://parallel.example/data/%d", size)

	// Prepare fake transport resource metadata once for logging consistency.
	resourceETag := fmt.Sprintf(`"test-file-%d"`, size)

	// Compute expected hash.
	expectedHash := computeExpectedHash(size)

	t.Run("Sequential", func(t *testing.T) {
		transport := testutil.NewFakeTransport()
		addLargeFileResource(transport, url, size)
		client := &http.Client{Transport: transport}

		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", url, err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("ETag") != resourceETag {
			t.Errorf("Expected ETag %s, got %s", resourceETag, resp.Header.Get("ETag"))
		}

		if resp.ContentLength != size {
			t.Errorf("Expected Content-Length %d, got %d",
				size, resp.ContentLength)
		}

		hashingReader := newHashingReader(resp.Body)
		_, err = io.Copy(io.Discard, hashingReader)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if hashingReader.BytesRead() != size {
			t.Errorf("Expected to read %d bytes, actually read %d bytes",
				size, hashingReader.BytesRead())
		}

		actualHash := hashingReader.Sum()
		if !bytes.Equal(expectedHash, actualHash) {
			t.Errorf("Hash mismatch.\nExpected: %x\nActual:   %x",
				expectedHash, actualHash)
		}
	})

	t.Run("Parallel", func(t *testing.T) {
		baseTransport := testutil.NewFakeTransport()
		addLargeFileResource(baseTransport, url, size)
		transport := New(
			baseTransport,
			WithMaxConcurrentPerHost(map[string]uint{"": 0}),
			WithMinChunkSize(4*1024*1024), // 4MB chunks.
			WithMaxConcurrentPerRequest(8),
		)
		client := &http.Client{Transport: transport}

		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", url, err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("ETag") != resourceETag {
			t.Errorf("Expected ETag %s, got %s", resourceETag, resp.Header.Get("ETag"))
		}

		if resp.ContentLength != size {
			t.Errorf("Expected Content-Length %d, got %d",
				size, resp.ContentLength)
		}

		hashingReader := newHashingReader(resp.Body)
		_, err = io.Copy(io.Discard, hashingReader)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if hashingReader.BytesRead() != size {
			t.Errorf("Expected to read %d bytes, actually read %d bytes",
				size, hashingReader.BytesRead())
		}

		actualHash := hashingReader.Sum()
		if !bytes.Equal(expectedHash, actualHash) {
			t.Errorf("Hash mismatch.\nExpected: %x\nActual:   %x",
				expectedHash, actualHash)
		}
	})
}

// TestVeryLargeFile_ParallelDownload tests parallel download of a very large
// file. The actual file size adapts based on whether the race detector is
// enabled (400MB in race mode, 4GB normally).
func TestVeryLargeFile_ParallelDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping very large file test in short mode")
	}

	// Test with very large file (4GB normally, 400MB in race/coverage mode).
	baseSize := int64(4 * 1024 * 1024 * 1024) // 4 GB base size.
	size := getTestFileSize(baseSize)

	if size != baseSize {
		t.Logf("Running with reduced file size: %d MB (race detector or coverage mode detected)",
			size/(1024*1024))
	}

	url := fmt.Sprintf("https://parallel.example/very-large/%d", size)

	baseTransport := testutil.NewFakeTransport()
	addLargeFileResource(baseTransport, url, size)

	// Only test parallel for very large files due to time constraints.
	transport := New(
		baseTransport,
		WithMaxConcurrentPerHost(map[string]uint{"": 0}),
		WithMinChunkSize(8*1024*1024), // 8MB chunks.
		WithMaxConcurrentPerRequest(16),
	)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.ContentLength != size {
		t.Errorf("Expected Content-Length %d, got %d",
			size, resp.ContentLength)
	}

	// For 4GB, let's just verify we can read the correct number of bytes.
	// Computing the full hash would take too long.
	bytesRead := int64(0)
	buf := make([]byte, 64*1024) // 64KB buffer.
	for {
		n, err := resp.Body.Read(buf)
		bytesRead += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
	}

	if bytesRead != size {
		t.Errorf("Expected to read %d bytes, actually read %d bytes",
			size, bytesRead)
	}

	t.Logf("Successfully read %d bytes (4GB) from parallel download",
		bytesRead)
}
