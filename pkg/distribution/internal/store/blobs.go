package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
)

const (
	blobsDir = "blobs"
)

var allowedAlgorithms = map[string]int{
	"sha256": 64,
	"sha512": 128,
}

func isSafeAlgorithm(a string) (int, bool) {
	hexLength, ok := allowedAlgorithms[a]
	return hexLength, ok
}

func isSafeHex(hexLength int, s string) bool {
	if len(s) != hexLength {
		return false
	}
	for _, c := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, c) {
			return false
		}
	}
	return true
}

// validateHash ensures the hash components are safe for filesystem paths
func validateHash(hash oci.Hash) error {
	hexLength, ok := isSafeAlgorithm(hash.Algorithm)
	if !ok {
		return fmt.Errorf("invalid hash algorithm: %q not in allowlist", hash.Algorithm)
	}
	if !isSafeHex(hexLength, hash.Hex) {
		return fmt.Errorf("invalid hash hex: contains non-hexadecimal characters or invalid length")
	}
	return nil
}

// blobDir returns the path to the blobs directory
func (s *LocalStore) blobsDir() string {
	return filepath.Join(s.rootPath, blobsDir)
}

// blobPath returns the path to the blob for the given hash.
func (s *LocalStore) blobPath(hash oci.Hash) (string, error) {
	if err := validateHash(hash); err != nil {
		return "", fmt.Errorf("unsafe hash: %w", err)
	}

	path := filepath.Join(s.rootPath, blobsDir, hash.Algorithm, hash.Hex)

	cleanRootPath := filepath.Clean(s.rootPath)
	cleanPath := filepath.Clean(path)
	relPath, err := filepath.Rel(cleanRootPath, cleanPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path traversal attempt detected: %s", path)
	}

	return cleanPath, nil
}

type blob interface {
	DiffID() (oci.Hash, error)
	Uncompressed() (io.ReadCloser, error)
}

// writeLayer writes the layer blob to the store.
// It returns true when a new blob was created and the blob's DiffID.
func (s *LocalStore) writeLayer(layer blob, updates chan<- oci.Update, rangeSuccess *remote.RangeSuccess) (bool, oci.Hash, error) {
	hash, err := layer.DiffID()
	if err != nil {
		return false, oci.Hash{}, fmt.Errorf("get file hash: %w", err)
	}

	// Also get the layer digest for Range header matching
	// (for remote layers, DiffID == Digest, but we need the digest string for rangeSuccess lookup)
	var digestStr string
	if digester, ok := layer.(interface{ Digest() (oci.Hash, error) }); ok {
		if d, digestErr := digester.Digest(); digestErr == nil {
			digestStr = d.String()
		}
	}

	hasBlob, err := s.hasBlob(hash)
	if err != nil {
		return false, oci.Hash{}, fmt.Errorf("check blob existence: %w", err)
	}
	if hasBlob {
		// TODO: write something to the progress channel (we probably need to redo progress reporting a little bit)
		return false, hash, nil
	}

	// Check if we're resuming an incomplete download
	incompleteSize, err := s.GetIncompleteSize(hash)
	if err != nil {
		return false, oci.Hash{}, fmt.Errorf("check incomplete size: %w", err)
	}

	lr, err := layer.Uncompressed()
	if err != nil {
		return false, oci.Hash{}, fmt.Errorf("get blob contents: %w", err)
	}
	defer lr.Close()

	// Also get the layer digest for Range header matching
	// (for remote layers, we need the digest string for rangeSuccess lookup)
	layerDigestStr := digestStr // preserve the original digestStr parameter
	if digester, ok := layer.(interface{ Digest() (oci.Hash, error) }); ok {
		if d, digestLayerErr := digester.Digest(); digestLayerErr == nil {
			layerDigestStr = d.String()
		}
	}

	// Wrap the reader with progress reporting, accounting for already downloaded bytes
	var r io.Reader
	if incompleteSize > 0 {
		r = progress.NewReaderWithOffset(lr, updates, incompleteSize)
	} else {
		r = progress.NewReader(lr, updates)
	}

	// WriteBlob will handle appending to incomplete files
	// The HTTP layer will handle resuming via Range headers
	if err := s.WriteBlobWithResume(hash, r, layerDigestStr, rangeSuccess); err != nil {
		return false, hash, err
	}
	return true, hash, nil
}

// WriteBlob writes the blob to the store. For backwards compatibility, this version
// does not support resume detection. Use WriteBlobWithResume for resume support.
func (s *LocalStore) WriteBlob(diffID oci.Hash, r io.Reader) error {
	return s.WriteBlobWithResume(diffID, r, "", nil)
}

// WriteBlobWithResume writes the blob to the store with optional resume support.
// If digestStr and rangeSuccess are provided, and rangeSuccess indicates a successful
// Range request for this digest, WriteBlob will append to the incomplete file instead
// of starting fresh.
func (s *LocalStore) WriteBlobWithResume(diffID oci.Hash, r io.Reader, digestStr string, rangeSuccess *remote.RangeSuccess) error {
	hasBlob, err := s.hasBlob(diffID)
	if err != nil {
		return fmt.Errorf("check blob existence: %w", err)
	}
	if hasBlob {
		return nil
	}

	path, err := s.blobPath(diffID)
	if err != nil {
		return fmt.Errorf("get blob path: %w", err)
	}

	incompletePath := incompletePath(path)

	// Check if we're resuming a partial download
	var f *os.File
	if stat, err := os.Stat(incompletePath); err == nil {
		existingSize := stat.Size()

		// Before resuming, verify that the incomplete file isn't already complete
		existingFile, openErr := os.Open(incompletePath)
		if openErr != nil {
			return fmt.Errorf("open incomplete file for verification: %w", openErr)
		}

		computedHash, _, sha256Err := oci.SHA256(existingFile)
		existingFile.Close()

		if sha256Err == nil && computedHash.String() == diffID.String() {
			// File is already complete, just rename it
			if renameErr := os.Rename(incompletePath, path); renameErr != nil {
				return fmt.Errorf("rename completed blob file: %w", renameErr)
			}
			return nil
		}

		// The HTTP request is made lazily. Read first byte to trigger the request.
		buf := make([]byte, 1)
		n, readErr := r.Read(buf)
		if readErr != nil && readErr != io.EOF {
			// Clean up the incomplete file on read error (unless it's a context cancellation
			// which should preserve the file for future resume attempts)
			if !errors.Is(readErr, context.Canceled) && !errors.Is(readErr, context.DeadlineExceeded) {
				_ = os.Remove(incompletePath)
			}
			return fmt.Errorf("read first byte: %w", readErr)
		}

		// Check if a Range request succeeded for this digest
		shouldResume := false
		if rangeSuccess != nil && digestStr != "" {
			if offset, ok := rangeSuccess.Get(digestStr); ok && offset == existingSize {
				shouldResume = true
			}
		}

		if shouldResume {
			// Range request succeeded and offset matches - append to incomplete file
			var openFileErr error
			f, openFileErr = os.OpenFile(incompletePath, os.O_APPEND|os.O_WRONLY, 0644)
			if openFileErr != nil {
				return fmt.Errorf("open incomplete file for resume: %w", openFileErr)
			}
		} else {
			// No Range success or offset mismatch - start fresh
			if removeErr := os.Remove(incompletePath); removeErr != nil {
				return fmt.Errorf("remove incomplete file: %w", removeErr)
			}
			var createErr error
			f, createErr = createFile(incompletePath)
			if createErr != nil {
				return fmt.Errorf("create blob file: %w", createErr)
			}
		}

		// Write the first byte we already read
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return fmt.Errorf("write first byte: %w", err)
			}
		}
		if readErr == io.EOF {
			// Only one byte in the entire response, we're done
			f.Close()
			if renameErr := os.Rename(incompletePath, path); renameErr != nil {
				return fmt.Errorf("rename blob file: %w", renameErr)
			}
			os.Remove(incompletePath)
			return nil
		}
	} else {
		// No incomplete file exists - create new file
		f, err = createFile(incompletePath)
		if err != nil {
			return fmt.Errorf("create blob file: %w", err)
		}
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		// On copy failure, only delete the incomplete file if it's not a context
		// cancellation. Context cancellation is a normal interruption and the file
		// should be preserved for future download attempts.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			_ = os.Remove(incompletePath)
		}
		return fmt.Errorf("copy blob %q to store: %w", diffID.String(), err)
	}

	f.Close() // Rename will fail on Windows if the file is still open.

	if renameFinalErr := os.Rename(incompletePath, path); renameFinalErr != nil {
		return fmt.Errorf("rename blob file: %w", renameFinalErr)
	}

	// Safety cleanup in case rename didn't remove the source
	os.Remove(incompletePath)
	return nil
}

// removeBlob removes the blob with the given hash from the store.
func (s *LocalStore) removeBlob(hash oci.Hash) error {
	path, err := s.blobPath(hash)
	if err != nil {
		return fmt.Errorf("get blob path: %w", err)
	}
	return os.Remove(path)
}

func (s *LocalStore) hasBlob(hash oci.Hash) (bool, error) {
	path, err := s.blobPath(hash)
	if err != nil {
		return false, fmt.Errorf("get blob path: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}
	return false, nil
}

// GetIncompleteSize returns the size of an incomplete blob if it exists, or 0 if it doesn't.
func (s *LocalStore) GetIncompleteSize(hash oci.Hash) (int64, error) {
	path, err := s.blobPath(hash)
	if err != nil {
		return 0, fmt.Errorf("get blob path: %w", err)
	}

	incompletePath := incompletePath(path)
	stat, err := os.Stat(incompletePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat incomplete file: %w", err)
	}

	return stat.Size(), nil
}

// createFile is a wrapper around os.Create that creates any parent directories as needed.
func createFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return nil, fmt.Errorf("create parent directory %q: %w", filepath.Dir(path), err)
	}
	return os.Create(path)
}

// incompletePath returns the path to the incomplete file for the given path.
func incompletePath(path string) string {
	return path + ".incomplete"
}

// writeConfigFile writes the model config JSON file to the blob store and reports whether the file was newly created.
func (s *LocalStore) writeConfigFile(mdl oci.Image) (bool, error) {
	hash, err := mdl.ConfigName()
	if err != nil {
		return false, fmt.Errorf("get digest: %w", err)
	}
	hasBlob, err := s.hasBlob(hash)
	if err != nil {
		return false, fmt.Errorf("check config existence: %w", err)
	}
	if hasBlob {
		return false, nil
	}

	path, err := s.blobPath(hash)
	if err != nil {
		return false, fmt.Errorf("get blob path: %w", err)
	}

	rcf, err := mdl.RawConfigFile()
	if err != nil {
		return false, fmt.Errorf("get raw manifest: %w", err)
	}
	if err := writeFile(path, rcf); err != nil {
		return false, err
	}
	return true, nil
}

// CleanupStaleIncompleteFiles removes incomplete download files that haven't been modified
// for more than the specified duration. This prevents disk space leaks from abandoned downloads.
func (s *LocalStore) CleanupStaleIncompleteFiles(maxAge time.Duration) error {
	blobsPath := s.blobsDir()
	if _, err := os.Stat(blobsPath); os.IsNotExist(err) {
		// Blobs directory doesn't exist yet, nothing to clean up
		return nil
	}

	var cleanedCount int
	var cleanupErrors []error

	// Walk through the blobs directory looking for .incomplete files
	err := filepath.Walk(blobsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Continue walking even if we encounter errors on individual files
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process .incomplete files
		if !strings.HasSuffix(path, ".incomplete") {
			return nil
		}

		// Check if file is older than maxAge
		if time.Since(info.ModTime()) > maxAge {
			if removeErr := os.Remove(path); removeErr != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove stale incomplete file %s: %w", path, removeErr))
			} else {
				cleanedCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking blobs directory: %w", err)
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("encountered %d errors during cleanup (cleaned %d files): %w", len(cleanupErrors), cleanedCount, cleanupErrors[0])
	}

	if cleanedCount > 0 {
		fmt.Printf("Cleaned up %d stale incomplete download file(s)\n", cleanedCount)
	}

	return nil
}
