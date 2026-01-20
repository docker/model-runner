package huggingface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
)

// Downloader manages file downloads from HuggingFace repositories
type Downloader struct {
	client   *Client
	repo     string
	revision string
	tempDir  string
}

// NewDownloader creates a new downloader for a HuggingFace repository
func NewDownloader(client *Client, repo, revision, tempDir string) *Downloader {
	if revision == "" {
		revision = "main"
	}
	return &Downloader{
		client:   client,
		repo:     repo,
		revision: revision,
		tempDir:  tempDir,
	}
}

// DownloadResult contains the result of downloading files
type DownloadResult struct {
	// LocalPaths maps original repo paths to local file paths
	LocalPaths map[string]string
	// TotalBytes is the total number of bytes downloaded
	TotalBytes int64
}

// syncWriter wraps an io.Writer with a mutex for thread-safe concurrent writes
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// fileIDFromPath generates a unique ID for a file based on its path
// Returns a sha256: prefixed hash to match layer ID format
func fileIDFromPath(path string) string {
	hash := sha256.Sum256([]byte(path))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// DownloadAll downloads all specified files with progress reporting
// Files are downloaded in parallel with per-file progress updates written to progressWriter
func (d *Downloader) DownloadAll(ctx context.Context, files []RepoFile, progressWriter io.Writer) (*DownloadResult, error) {
	if len(files) == 0 {
		return &DownloadResult{LocalPaths: make(map[string]string)}, nil
	}

	totalSize := TotalSize(files)

	// Create result map (thread-safe access)
	var mu sync.Mutex
	localPaths := make(map[string]string, len(files))

	// Create thread-safe writer for concurrent progress reporting
	var safeWriter io.Writer
	if progressWriter != nil {
		safeWriter = &syncWriter{w: progressWriter}
	}

	// Download files in parallel (limit concurrency to avoid overwhelming)
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	errChan := make(chan error, len(files))

	for _, file := range files {
		wg.Add(1)
		go func(f RepoFile) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
			defer func() { <-sem }()

			localPath, err := d.downloadFileWithProgress(ctx, f, uint64(totalSize), safeWriter)
			if err != nil {
				errChan <- fmt.Errorf("download %s: %w", f.Path, err)
				return
			}

			mu.Lock()
			localPaths[f.Path] = localPath
			mu.Unlock()
		}(file)
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("download errors: %v", errs)
	}

	// Calculate total downloaded
	var totalDownloaded int64
	for _, f := range files {
		totalDownloaded += f.ActualSize()
	}

	return &DownloadResult{
		LocalPaths: localPaths,
		TotalBytes: totalDownloaded,
	}, nil
}

// downloadFileWithProgress downloads a single file with progress reporting
func (d *Downloader) downloadFileWithProgress(ctx context.Context, file RepoFile, totalImageSize uint64, progressWriter io.Writer) (string, error) {
	// Create local file path (preserve directory structure)
	localPath := filepath.Join(d.tempDir, file.Path)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	// Download from HuggingFace
	reader, _, err := d.client.DownloadFile(ctx, d.repo, d.revision, file.Path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// Create local file
	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Generate unique ID for this file (for progress tracking)
	fileID := fileIDFromPath(file.Path)
	fileSize := uint64(file.ActualSize())

	// Copy with progress tracking
	pr := &progressReader{
		reader:         reader,
		progressWriter: progressWriter,
		totalImageSize: totalImageSize,
		fileSize:       fileSize,
		fileID:         fileID,
	}

	if _, err := io.Copy(f, pr); err != nil {
		os.Remove(localPath) // Clean up on error
		return "", fmt.Errorf("write file: %w", err)
	}

	// Write final progress for this file (100% complete)
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "", totalImageSize, fileSize, fileSize, fileID, "pull")
	}

	return localPath, nil
}

// progressReader wraps a reader and reports per-file progress
type progressReader struct {
	reader         io.Reader
	progressWriter io.Writer
	totalImageSize uint64
	fileSize       uint64
	fileID         string
	bytesRead      uint64
	lastReported   uint64
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		pr.bytesRead += uint64(n)

		// Report progress periodically (every 1MB or when complete)
		if pr.progressWriter != nil && (pr.bytesRead-pr.lastReported >= progress.MinBytesForUpdate || pr.bytesRead == pr.fileSize) {
			_ = progress.WriteProgress(pr.progressWriter, "", pr.totalImageSize, pr.fileSize, pr.bytesRead, pr.fileID, "pull")
			pr.lastReported = pr.bytesRead
		}
	}
	return n, err
}

// DownloadSingleFile downloads a single file and returns its local path
func (d *Downloader) DownloadSingleFile(ctx context.Context, file RepoFile) (string, error) {
	return d.downloadFileWithProgress(ctx, file, uint64(file.ActualSize()), nil)
}
