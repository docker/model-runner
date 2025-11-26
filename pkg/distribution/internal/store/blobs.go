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

	"github.com/docker/model-runner/pkg/distribution/internal/progress"

	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
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
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// validateHash ensures the hash components are safe for filesystem paths
func validateHash(hash v1.Hash) error {
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
func (s *LocalStore) blobPath(hash v1.Hash) (string, error) {
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
	DiffID() (v1.Hash, error)
	Uncompressed() (io.ReadCloser, error)
}

// layerWithDigest extends blob to include the Digest method
type layerWithDigest interface {
	blob
	Digest() (v1.Hash, error)
}

// writeLayer writes the layer blob to the store.
// It returns true when a new blob was created and the blob's DiffID.
func (s *LocalStore) writeLayer(layer blob, updates chan<- v1.Update) (bool, v1.Hash, error) {
	hash, err := layer.DiffID()
	if err != nil {
		return false, v1.Hash{}, fmt.Errorf("get file hash: %w", err)
	}
	hasBlob, err := s.hasBlob(hash)
	if err != nil {
		return false, v1.Hash{}, fmt.Errorf("check blob existence: %w", err)
	}
	if hasBlob {
		// TODO: write something to the progress channel (we probably need to redo progress reporting a little bit)
		return false, hash, nil
	}

	// Check if we're resuming an incomplete download
	incompleteSize, err := s.GetIncompleteSize(hash)
	if err != nil {
		return false, v1.Hash{}, fmt.Errorf("check incomplete size: %w", err)
	}

	lr, err := layer.Uncompressed()
	if err != nil {
		return false, v1.Hash{}, fmt.Errorf("get blob contents: %w", err)
	}
	defer lr.Close()

	// Wrap the reader with progress reporting, accounting for already downloaded bytes
	var r io.Reader
	if incompleteSize > 0 {
		r = progress.NewReaderWithOffset(lr, updates, incompleteSize)
	} else {
		r = progress.NewReader(lr, updates)
	}

	// WriteBlob will handle appending to incomplete files
	// The HTTP layer will handle resuming via Range headers
	if err := s.WriteBlob(hash, r); err != nil {
		return false, hash, err
	}
	return true, hash, nil
}

// WriteBlob writes the blob to the store, reporting progress to the given channel.
// If the blob is already in the store, it is a no-op and the blob is not consumed from the reader.
// If an incomplete download exists, it will be resumed by appending to the existing file.
func (s *LocalStore) WriteBlob(diffID v1.Hash, r io.Reader) error {
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
	var isResume bool
	if _, err := os.Stat(incompletePath); err == nil {
		// Before resuming, verify that the incomplete file isn't already complete
		existingFile, err := os.Open(incompletePath)
		if err != nil {
			return fmt.Errorf("open incomplete file for verification: %w", err)
		}

		computedHash, _, err := v1.SHA256(existingFile)
		existingFile.Close()

		if err == nil && computedHash.String() == diffID.String() {
			// File is already complete, just rename it
			if err := os.Rename(incompletePath, path); err != nil {
				return fmt.Errorf("rename completed blob file: %w", err)
			}
			return nil
		}

		// File is incomplete or corrupt, try to resume
		isResume = true
		f, err = os.OpenFile(incompletePath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open incomplete blob file for resume: %w", err)
		}
	} else {
		// New download: create file
		f, err = createFile(incompletePath)
		if err != nil {
			return fmt.Errorf("create blob file: %w", err)
		}
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		// If we were resuming and copy failed, only delete the incomplete file if it's
		// not a context cancellation. Context cancellation is a normal interruption and
		// the file should be preserved for future resume attempts.
		if isResume && !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			_ = os.Remove(incompletePath)
		}
		return fmt.Errorf("copy blob %q to store: %w", diffID.String(), err)
	}

	f.Close() // Rename will fail on Windows if the file is still open.

	// For resumed downloads, verify the complete file's hash before finalizing
	// (For new downloads, the stream was already verified during download)
	if isResume {
		completeFile, err := os.Open(incompletePath)
		if err != nil {
			return fmt.Errorf("open completed file for verification: %w", err)
		}
		defer completeFile.Close()

		computedHash, _, err := v1.SHA256(completeFile)
		if err != nil {
			return fmt.Errorf("compute hash of completed file: %w", err)
		}

		if computedHash.String() != diffID.String() {
			// The resumed download is corrupt, remove it so we can start fresh next time
			_ = os.Remove(incompletePath)
			return fmt.Errorf("hash mismatch after download: got %s, want %s", computedHash, diffID)
		}
	}

	if err := os.Rename(incompletePath, path); err != nil {
		return fmt.Errorf("rename blob file: %w", err)
	}

	// Only remove incomplete file if rename succeeded (though rename should have moved it)
	// This is a safety cleanup in case rename didn't remove the source
	os.Remove(incompletePath)
	return nil
}

// removeBlob removes the blob with the given hash from the store.
func (s *LocalStore) removeBlob(hash v1.Hash) error {
	path, err := s.blobPath(hash)
	if err != nil {
		return fmt.Errorf("get blob path: %w", err)
	}
	return os.Remove(path)
}

func (s *LocalStore) hasBlob(hash v1.Hash) (bool, error) {
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
func (s *LocalStore) GetIncompleteSize(hash v1.Hash) (int64, error) {
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
func (s *LocalStore) writeConfigFile(mdl v1.Image) (bool, error) {
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
		return fmt.Errorf("encountered %d errors during cleanup (cleaned %d files): %v", len(cleanupErrors), cleanedCount, cleanupErrors[0])
	}

	if cleanedCount > 0 {
		fmt.Printf("Cleaned up %d stale incomplete download file(s)\n", cleanedCount)
	}

	return nil
}
