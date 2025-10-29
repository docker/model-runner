package testing

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"
)

// GenerateTestData generates deterministic test data of the specified size.
func GenerateTestData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// GenerateRandomData generates random test data of the specified size.
func GenerateRandomData(size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("failed to generate random data: %v", err))
	}
	return data
}

// AssertDataEquals checks if two byte slices are equal.
func AssertDataEquals(t *testing.T, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Errorf("data mismatch: got %d bytes, want %d bytes", len(got), len(want))
		if len(got) == len(want) {
			// Find first difference.
			for i := range got {
				if got[i] != want[i] {
					t.Errorf(
						"first difference at byte %d: got %02x, want %02x",
						i, got[i], want[i])
					break
				}
			}
		}
	}
}

// ReadAll reads all data from a reader and returns it.
func ReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read all data: %v", err)
	}
	return data
}

// ReadAllWithError reads all data from a reader and returns both data and
// error.
func ReadAllWithError(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// MustRead reads exactly n bytes from a reader or fails the test.
func MustRead(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	nn, err := io.ReadFull(r, buf)
	if err != nil {
		t.Fatalf(
			"failed to read %d bytes: got %d, err: %v", n, nn, err)
	}
	return buf
}

// AssertHeaderEquals checks if a header has the expected value.
func AssertHeaderEquals(t *testing.T, headers map[string][]string, key, want string) {
	t.Helper()
	values, ok := headers[key]
	if !ok || len(values) == 0 {
		if want != "" {
			t.Errorf("header %q not found, want %q", key, want)
		}
		return
	}
	if values[0] != want {
		t.Errorf("header %q = %q, want %q", key, values[0], want)
	}
}

// AssertHeaderPresent checks if a header is present.
func AssertHeaderPresent(t *testing.T, headers map[string][]string, key string) {
	t.Helper()
	if _, ok := headers[key]; !ok {
		t.Errorf("header %q not found", key)
	}
}

// AssertHeaderAbsent checks if a header is absent.
func AssertHeaderAbsent(t *testing.T, headers map[string][]string, key string) {
	t.Helper()
	if _, ok := headers[key]; ok {
		t.Errorf("header %q found, want absent", key)
	}
}

// ChunkData splits data into n chunks of approximately equal size.
func ChunkData(data []byte, n int) [][]byte {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return [][]byte{data}
	}

	chunkSize := len(data) / n
	remainder := len(data) % n

	chunks := make([][]byte, n)
	offset := 0

	for i := 0; i < n; i++ {
		size := chunkSize
		if i == n-1 {
			size += remainder
		}
		chunks[i] = data[offset : offset+size]
		offset += size
	}

	return chunks
}

// ConcatChunks concatenates multiple byte slices into one.
func ConcatChunks(chunks [][]byte) []byte {
	var total int
	for _, chunk := range chunks {
		total += len(chunk)
	}

	result := make([]byte, 0, total)
	for _, chunk := range chunks {
		result = append(result, chunk...)
	}

	return result
}

// ByteRange represents a byte range.
type ByteRange struct {
	// Start is the starting byte position (inclusive).
	Start int64
	// End is the ending byte position (inclusive).
	End int64
}

// CalculateByteRanges calculates byte ranges for splitting a file of given
// size into n parts.
func CalculateByteRanges(totalSize int64, n int) []ByteRange {
	if n <= 0 || totalSize <= 0 {
		return nil
	}

	ranges := make([]ByteRange, n)
	chunkSize := totalSize / int64(n)
	remainder := totalSize % int64(n)

	var start int64
	for i := 0; i < n; i++ {
		size := chunkSize
		if i == n-1 {
			size += remainder
		}
		ranges[i] = ByteRange{
			Start: start,
			End:   start + size - 1,
		}
		start += size
	}

	return ranges
}

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error, got nil", msg)
	}
}
