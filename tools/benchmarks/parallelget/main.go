package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/docker/model-runner/pkg/distribution/transport/parallel"
)

var (
	minChunkSize  int64
	maxConcurrent uint
)

var rootCmd = &cobra.Command{
	Use:   "parallelget <url>",
	Short: "Benchmark parallel vs non-parallel HTTP GET requests",
	Long: `parallelget is a benchmarking tool that compares the performance of standard
HTTP GET requests against parallelized requests using the transport/parallel package.

It downloads the same URL twice - once using the standard HTTP client and once
using a parallel transport - then compares the results and reports performance metrics.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runBenchmark,
	SilenceUsage: true,
}

func init() {
	rootCmd.Flags().Int64Var(&minChunkSize, "chunk-size", 1024*1024, "Minimum chunk size in bytes for parallelization (default 1MB)")
	rootCmd.Flags().UintVar(&maxConcurrent, "max-concurrent", 4, "Maximum concurrent requests for parallel transport (default 4)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	url := args[0]

	fmt.Printf("Benchmarking HTTP GET performance for: %s\n", url)
	fmt.Printf("Configuration: chunk-size=%d bytes, max-concurrent=%d\n\n", minChunkSize, maxConcurrent)

	// Create temporary files for storing responses.
	nonParallelFile, err := os.CreateTemp("", "benchmark-non-parallel-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file for non-parallel response: %w", err)
	}
	defer func() {
		nonParallelFile.Close()
		os.Remove(nonParallelFile.Name())
	}()

	parallelFile, err := os.CreateTemp("", "benchmark-parallel-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file for parallel response: %w", err)
	}
	defer func() {
		parallelFile.Close()
		os.Remove(parallelFile.Name())
	}()

	// Run non-parallel benchmark.
	fmt.Println("Running non-parallel benchmark...")
	nonParallelDuration, nonParallelSize, err := benchmarkNonParallel(url, nonParallelFile)
	if err != nil {
		return fmt.Errorf("non-parallel benchmark failed: %w", err)
	}
	fmt.Printf("✓ Non-parallel: %d bytes in %v (%.2f MB/s)\n", nonParallelSize, nonParallelDuration,
		float64(nonParallelSize)/nonParallelDuration.Seconds()/(1024*1024))

	// Run parallel benchmark.
	fmt.Println("Running parallel benchmark...")
	parallelDuration, parallelSize, err := benchmarkParallel(url, parallelFile)
	if err != nil {
		return fmt.Errorf("parallel benchmark failed: %w", err)
	}
	fmt.Printf("✓ Parallel: %d bytes in %v (%.2f MB/s)\n", parallelSize, parallelDuration,
		float64(parallelSize)/parallelDuration.Seconds()/(1024*1024))

	// Validate responses match.
	fmt.Println("Validating response consistency...")
	if err := validateResponses(nonParallelFile, parallelFile); err != nil {
		return fmt.Errorf("response validation failed: %w", err)
	}
	fmt.Println("✓ Responses match perfectly")

	// Print performance comparison.
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PERFORMANCE COMPARISON")
	fmt.Println(strings.Repeat("=", 60))

	speedup := float64(nonParallelDuration) / float64(parallelDuration)
	if speedup > 1.0 {
		fmt.Printf("🚀 Parallel was %.2fx faster than non-parallel\n", speedup)
		timeSaved := nonParallelDuration - parallelDuration
		fmt.Printf("⏱️  Time saved: %v (%.1f%%)\n", timeSaved, (1.0-1.0/speedup)*100)
	} else if speedup < 1.0 {
		slowdown := 1.0 / speedup
		fmt.Printf("⚠️  Parallel was %.2fx slower than non-parallel\n", slowdown)
		fmt.Printf("⏱️  Time penalty: %v (%.1f%%)\n", parallelDuration-nonParallelDuration, (slowdown-1.0)*100)
	} else {
		fmt.Println("📊 Both approaches performed equally")
	}

	fmt.Printf("\nDetailed timing:\n")
	fmt.Printf("  Non-parallel: %v\n", nonParallelDuration)
	fmt.Printf("  Parallel:     %v\n", parallelDuration)
	fmt.Printf("  Difference:   %v\n", parallelDuration-nonParallelDuration)

	return nil
}

// performHTTPGet executes an HTTP GET request using the specified transport
// and measures the time taken to download the entire response body.
// The response is written to outputFile and progress is displayed during the download.
func performHTTPGet(url string, transport http.RoundTripper, outputFile *os.File) (time.Duration, int64, error) {
	client := &http.Client{
		Transport: transport,
	}

	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create progress writer with content length if available.
	contentLength := resp.ContentLength
	if contentLength <= 0 {
		contentLength = -1 // Unknown size.
	}
	progressWriter := newProgressWriter(outputFile, contentLength, "  Progress")

	written, err := io.Copy(progressWriter, resp.Body)
	progressWriter.finish() // Ensure final progress is shown.

	if err != nil {
		return 0, 0, err
	}

	duration := time.Since(start)
	return duration, written, nil
}

// benchmarkNonParallel performs a standard HTTP GET request using the default transport
// and measures the time taken to download the entire response body.
// The response is written to outputFile and progress is displayed during the download.
func benchmarkNonParallel(url string, outputFile *os.File) (time.Duration, int64, error) {
	return performHTTPGet(url, http.DefaultTransport, outputFile)
}

// benchmarkParallel performs an HTTP GET request using the parallel transport
// and measures the time taken to download the entire response body.
// The parallel transport uses byte-range requests to download chunks concurrently.
// The response is written to outputFile and progress is displayed during the download.
func benchmarkParallel(url string, outputFile *os.File) (time.Duration, int64, error) {
	// Create parallel transport with configuration.
	parallelTransport := parallel.New(
		http.DefaultTransport,
		parallel.WithMaxConcurrentPerHost(map[string]uint{"": 0}),
		parallel.WithMinChunkSize(minChunkSize),
		parallel.WithMaxConcurrentPerRequest(maxConcurrent),
	)

	return performHTTPGet(url, parallelTransport, outputFile)
}

func validateResponses(file1, file2 *os.File) error {
	// Get file sizes first for quick comparison.
	stat1, err := file1.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat non-parallel file: %w", err)
	}

	stat2, err := file2.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat parallel file: %w", err)
	}

	// Compare file sizes - if they differ, no need to compute hashes.
	if stat1.Size() != stat2.Size() {
		return fmt.Errorf("file sizes differ: non-parallel=%d bytes, parallel=%d bytes",
			stat1.Size(), stat2.Size())
	}

	// Compute SHA-256 hash for first file.
	hash1, err := computeFileHash(file1)
	if err != nil {
		return fmt.Errorf("failed to compute hash for non-parallel file: %w", err)
	}

	// Compute SHA-256 hash for second file.
	hash2, err := computeFileHash(file2)
	if err != nil {
		return fmt.Errorf("failed to compute hash for parallel file: %w", err)
	}

	// Compare the hashes.
	if !bytes.Equal(hash1, hash2) {
		return fmt.Errorf("file contents differ: SHA-256 hashes do not match")
	}

	return nil
}

// computeFileHash computes the SHA-256 hash of a file's contents.
// The file is read from the beginning using a single io.Copy operation for efficiency.
func computeFileHash(file *os.File) ([]byte, error) {
	// Seek to beginning of file.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to beginning: %w", err)
	}

	// Create SHA-256 hasher.
	hasher := sha256.New()

	// Copy entire file content to hasher in a single operation.
	_, err := io.Copy(hasher, file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file for hashing: %w", err)
	}

	// Return the computed hash.
	return hasher.Sum(nil), nil
}

// progressWriter wraps an io.Writer and provides progress updates during writes.
// It displays a progress bar with percentage completion and transfer rates,
// updating the display at regular intervals to avoid excessive output.
type progressWriter struct {
	// writer is the underlying writer to write data to.
	writer io.Writer
	// total is the total expected bytes (-1 if unknown).
	total int64
	// written is the number of bytes written so far.
	written int64
	// lastUpdate is the last time the progress display was updated.
	lastUpdate time.Time
	// label is the label to display with the progress bar.
	label string
	// finished indicates whether the progress display has been finalized.
	finished bool
	// mu protects concurrent access to progress state.
	mu sync.Mutex
}

// newProgressWriter creates a new progress writer that wraps the given writer.
// The total parameter specifies the expected number of bytes (use -1 if unknown).
// The label parameter is displayed alongside the progress bar.
func newProgressWriter(writer io.Writer, total int64, label string) *progressWriter {
	return &progressWriter{
		writer:     writer,
		total:      total,
		label:      label,
		lastUpdate: time.Now(),
	}
}

// Write implements io.Writer, writing data to the underlying writer and updating progress.
// Progress is displayed at most every 100ms to avoid overwhelming the terminal with updates.
// The final progress update is handled by the finish() method to ensure clean display.
func (pw *progressWriter) Write(data []byte) (int, error) {
	// Write data to the underlying writer first.
	n, err := pw.writer.Write(data)
	if n > 0 {
		pw.mu.Lock()
		pw.written += int64(n)
		now := time.Now()

		// Update progress every 100ms to balance responsiveness and performance.
		// Don't update on completion - let finish() handle the final display.
		if now.Sub(pw.lastUpdate) >= 100*time.Millisecond && (pw.total < 0 || pw.written < pw.total) {
			pw.printProgress()
			pw.lastUpdate = now
		}
		pw.mu.Unlock()
	}
	return n, err
}

// printProgress displays the current progress to the terminal.
// For files with known size, shows a progress bar with percentage and bytes.
// For files with unknown size, shows only the bytes transferred.
// Uses carriage return (\r) to overwrite the previous progress line.
func (pw *progressWriter) printProgress() {
	if pw.finished {
		return
	}

	if pw.total < 0 {
		// Unknown total size - just show bytes transferred.
		fmt.Printf("\r%s: %d bytes", pw.label, pw.written)
		return
	}

	// Calculate percentage, capping at 100% to handle edge cases.
	percent := float64(pw.written) / float64(pw.total) * 100
	if percent > 100 {
		percent = 100
	}

	// Create a visual progress bar using filled and empty characters.
	barWidth := 30
	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Display progress bar with percentage and byte counts.
	fmt.Printf("\r%s: [%s] %.1f%% (%d/%d bytes)",
		pw.label, bar, percent, pw.written, pw.total)
}

// finish completes the progress display by showing the final progress state
// and adding a newline to move the cursor to the next line.
// This ensures the progress bar doesn't interfere with subsequent output.
// It's safe to call multiple times - subsequent calls are ignored.
func (pw *progressWriter) finish() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if !pw.finished {
		// Display final progress state.
		pw.printProgress()
		// Move to next line to prevent interference with subsequent output.
		fmt.Println()
		pw.finished = true
	}
}
