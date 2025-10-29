package testing

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// TestFakeTransport_Basic tests the basic functionality of FakeTransport.
func TestFakeTransport_Basic(t *testing.T) {
	ft := NewFakeTransport()

	// Add a simple resource.
	data := []byte("Hello, World!")
	ft.AddSimple("http://example.com/test", bytes.NewReader(data), int64(len(data)), true)

	// Create a request.
	req, err := http.NewRequest("GET", "http://example.com/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Perform the request.
	resp, err := ft.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	// Read the response.
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check the data.
	if !bytes.Equal(got, data) {
		t.Errorf("Response data mismatch: got %q, want %q", got, data)
	}
}

// TestFlakyReader_FailsAfterN tests that FlakyReader fails after reading
// a specified number of bytes.
func TestFlakyReader_FailsAfterN(t *testing.T) {
	data := []byte("Hello, World!")
	fr := NewFlakyReader(bytes.NewReader(data), int64(len(data)), 5)

	// Read first 5 bytes.
	buf := make([]byte, 5)
	n, err := fr.Read(buf)
	if err != nil {
		t.Fatalf("First read failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf) != "Hello" {
		t.Errorf("Expected 'Hello', got %q", string(buf))
	}

	// Next read should fail.
	_, err = fr.Read(buf)
	if err != ErrFlakyFailure {
		t.Errorf("Expected ErrFlakyFailure, got %v", err)
	}
}

// TestHelpers_GenerateTestData tests the deterministic test data generator.
func TestHelpers_GenerateTestData(t *testing.T) {
	data := GenerateTestData(256)

	if len(data) != 256 {
		t.Errorf("Expected 256 bytes, got %d", len(data))
	}

	// Check deterministic pattern.
	for i := 0; i < 256; i++ {
		if data[i] != byte(i%256) {
			t.Errorf("Byte %d: expected %d, got %d", i, i%256, data[i])
		}
	}
}

// TestHelpers_ChunkData tests the data chunking functionality.
func TestHelpers_ChunkData(t *testing.T) {
	data := GenerateTestData(100)
	chunks := ChunkData(data, 4)

	if len(chunks) != 4 {
		t.Fatalf("Expected 4 chunks, got %d", len(chunks))
	}

	// First 3 chunks should be 25 bytes each.
	for i := 0; i < 3; i++ {
		if len(chunks[i]) != 25 {
			t.Errorf("Chunk %d: expected 25 bytes, got %d", i, len(chunks[i]))
		}
	}

	// Last chunk should be 25 + remainder.
	if len(chunks[3]) != 25 {
		t.Errorf("Last chunk: expected 25 bytes, got %d", len(chunks[3]))
	}

	// Concatenate and verify.
	combined := ConcatChunks(chunks)
	if !bytes.Equal(combined, data) {
		t.Error("Concatenated chunks don't match original data")
	}
}

// TestHelpers_ByteRanges tests byte range calculation for parallel
// downloads.
func TestHelpers_ByteRanges(t *testing.T) {
	ranges := CalculateByteRanges(100, 4)

	if len(ranges) != 4 {
		t.Fatalf("Expected 4 ranges, got %d", len(ranges))
	}

	expectedRanges := []ByteRange{
		{Start: 0, End: 24},
		{Start: 25, End: 49},
		{Start: 50, End: 74},
		{Start: 75, End: 99},
	}

	for i, r := range ranges {
		if r.Start != expectedRanges[i].Start ||
			r.End != expectedRanges[i].End {
			t.Errorf(
				"Range %d: got %d-%d, want %d-%d",
				i, r.Start, r.End, expectedRanges[i].Start, expectedRanges[i].End)
		}
	}
}
