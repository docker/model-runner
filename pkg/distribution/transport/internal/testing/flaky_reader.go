package testing

import (
	"errors"
	"io"
	"sync"
)

// ErrFlakyFailure is returned when FlakyReader simulates a failure.
var ErrFlakyFailure = errors.New("simulated read failure")

// FlakyReader simulates a reader that fails after a certain number of
// bytes.
type FlakyReader struct {
	// data holds the content to be read through random access reads.
	data io.ReaderAt
	// length is the total number of readable bytes.
	length int64
	// failAfter is the byte position after which reads should fail.
	failAfter int64
	// pos is the current read position.
	pos int64
	// failed indicates if the reader has already failed.
	failed bool
	// closed indicates if the reader has been closed.
	closed bool
	// mu protects all fields from concurrent access.
	mu sync.Mutex
}

// NewFlakyReader creates a FlakyReader that fails after reading failAfter
// bytes. If failAfter is 0 or negative, it never fails.
func NewFlakyReader(data io.ReaderAt, length int64, failAfter int) *FlakyReader {
	return &FlakyReader{
		data:      data,
		length:    length,
		failAfter: int64(failAfter),
	}
}

// Read implements io.Reader.
func (fr *FlakyReader) Read(p []byte) (int, error) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if fr.closed {
		return 0, errors.New("read from closed reader")
	}

	if fr.failed {
		return 0, ErrFlakyFailure
	}

	if fr.pos >= fr.length {
		return 0, io.EOF
	}

	// Calculate how much we can read.
	remaining := fr.length - fr.pos
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Check if we should fail.
	if fr.failAfter > 0 && fr.pos+toRead > fr.failAfter {
		toRead = fr.failAfter - fr.pos
		if toRead <= 0 {
			fr.failed = true
			return 0, ErrFlakyFailure
		}
	}

	if toRead == 0 {
		return 0, nil
	}

	buf := p[:toRead]
	n, err := fr.data.ReadAt(buf, fr.pos)
	fr.pos += int64(n)

	if err != nil && err != io.EOF {
		return n, err
	}

	if fr.failAfter > 0 && fr.pos >= fr.failAfter && fr.pos < fr.length {
		fr.failed = true
		if n == 0 {
			return 0, ErrFlakyFailure
		}
	}

	if fr.pos >= fr.length {
		return n, io.EOF
	}

	if err == io.EOF {
		return n, io.EOF
	}

	return n, nil
}

// Close implements io.Closer.
func (fr *FlakyReader) Close() error {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.closed = true
	return nil
}

// Reset resets the reader to start from the beginning.
func (fr *FlakyReader) Reset() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.pos = 0
	fr.failed = false
	fr.closed = false
}

// Position returns the current read position.
func (fr *FlakyReader) Position() int {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return int(fr.pos)
}

// HasFailed returns true if the reader has simulated a failure.
func (fr *FlakyReader) HasFailed() bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return fr.failed
}

// MultiFailReader simulates multiple failures at different points.
type MultiFailReader struct {
	// data holds the content to be read through random access reads.
	data io.ReaderAt
	// length is the total number of readable bytes.
	length int64
	// failurePoints are the byte positions where failures should occur.
	failurePoints []int
	// failureCount tracks how many failures have been simulated.
	failureCount int
	// pos is the current read position.
	pos int64
	// closed indicates if the reader has been closed.
	closed bool
	// mu protects all fields from concurrent access.
	mu sync.Mutex
}

// NewMultiFailReader creates a reader that fails at specified byte
// positions.
func NewMultiFailReader(data io.ReaderAt, length int64, failurePoints []int) *MultiFailReader {
	return &MultiFailReader{
		data:          data,
		length:        length,
		failurePoints: failurePoints,
	}
}

// Read implements io.Reader.
func (mfr *MultiFailReader) Read(p []byte) (int, error) {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()

	if mfr.closed {
		return 0, errors.New("read from closed reader")
	}

	if mfr.pos >= mfr.length {
		return 0, io.EOF
	}

	// Check if we're at a failure point.
	for i, point := range mfr.failurePoints {
		if i < mfr.failureCount {
			continue // Already failed here.
		}
		if mfr.pos == int64(point) {
			mfr.failureCount++
			return 0, ErrFlakyFailure
		}
	}

	// Calculate how much to read.
	remaining := mfr.length - mfr.pos
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Check if we would cross a failure point.
	for i, point := range mfr.failurePoints {
		if i < mfr.failureCount {
			continue // Skip already used failure points.
		}
		if mfr.pos < int64(point) && mfr.pos+toRead > int64(point) {
			toRead = int64(point) - mfr.pos
			break
		}
	}

	// Copy data.
	if toRead == 0 {
		return 0, nil
	}

	buf := p[:toRead]
	n, err := mfr.data.ReadAt(buf, mfr.pos)
	mfr.pos += int64(n)

	if err != nil && err != io.EOF {
		return n, err
	}

	if mfr.pos >= mfr.length {
		return n, io.EOF
	}

	if err == io.EOF {
		return n, io.EOF
	}

	return n, nil
}

// Close implements io.Closer.
func (mfr *MultiFailReader) Close() error {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()
	mfr.closed = true
	return nil
}

// Reset resets the reader to the beginning and clears failure state.
func (mfr *MultiFailReader) Reset() {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()
	mfr.pos = 0
	mfr.failureCount = 0
	mfr.closed = false
}
