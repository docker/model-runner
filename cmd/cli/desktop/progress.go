package desktop

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client/pkg/jsonmessage"
)

// DisplayProgress displays progress messages from a model pull/push operation
// using Docker-style multi-line progress bars.
// Returns the final message, whether progress was actually shown, and any error.
func DisplayProgress(
	body io.Reader, printer standalone.StatusPrinter,
) (finalMessage string, progressShown bool, retErr error) {
	fd, isTerminal := printer.GetFdInfo()

	// If not a terminal, fall back to simple line-by-line output
	if !isTerminal {
		return displayProgressSimple(body, printer)
	}

	// Use a pipe to convert our progress messages to Docker's JSONMessage format
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	// Start the display goroutine
	go func() {
		err := jsonmessage.DisplayJSONMessagesStream(pr, &writerAdapter{printer}, fd, isTerminal, nil)
		if err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Ensure the pipe is always closed and the display goroutine is always
	// drained, even on early returns, to prevent goroutine leaks.
	defer func() {
		pw.Close()
		if displayErr := <-errCh; retErr == nil &&
			displayErr != nil && !errors.Is(displayErr, io.EOF) {
			retErr = displayErr
		}
	}()

	// Convert progress messages to JSONMessage format
	scanner := bufio.NewScanner(body)
	// nonJSONBytes collects raw unparseable lines for error reporting,
	// capped at maxNonJSONBytes to avoid large allocations.
	var nonJSONBytes []byte
	var nonJSONTruncated bool

	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		var progressMsg oci.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			// Collect unparseable lines (e.g. HTML error pages from proxies)
			// so we can surface them if no valid progress arrives.
			nonJSONBytes, nonJSONTruncated = appendNonJSONLine(nonJSONBytes, progressLine)
			continue
		}

		switch progressMsg.Type {
		case oci.TypeProgress:
			progressShown = true // We're showing actual progress
			if err := writeDockerProgress(pw, &progressMsg); err != nil {
				return "", false, err
			}

		case oci.TypeSuccess:
			finalMessage = progressMsg.Message
			// Don't write the success message here - let the caller print it
			// to avoid duplicate output

		case oci.TypeWarning:
			// Print warning to stderr
			printer.PrintErrf("Warning: %s\n", progressMsg.Message)

		case oci.TypeError:
			return "", false, fmt.Errorf("%s", progressMsg.Message)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, err
	}

	// If we received only unparseable lines and no valid progress or success,
	// surface the raw content as an error. This catches HTML error pages
	// returned by proxies or CDNs in place of a proper progress stream.
	if finalMessage == "" && !progressShown {
		if err := unexpectedProgressDataError(nonJSONBytes, nonJSONTruncated); err != nil {
			return "", false, err
		}
	}

	return finalMessage, progressShown, nil
}

// displayProgressSimple displays progress messages in simple line-by-line format
func displayProgressSimple(body io.Reader, printer standalone.StatusPrinter) (string, bool, error) {
	scanner := bufio.NewScanner(body)
	var current uint64
	layerProgress := make(map[string]uint64)
	var finalMessage string
	progressShown := false // Track if we actually showed any progress
	// nonJSONBytes collects raw unparseable lines for error reporting.
	var nonJSONBytes []byte
	var nonJSONTruncated bool

	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		var progressMsg oci.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			// Collect unparseable lines for error reporting.
			nonJSONBytes, nonJSONTruncated = appendNonJSONLine(nonJSONBytes, progressLine)
			continue
		}

		switch progressMsg.Type {
		case oci.TypeProgress:
			progressShown = true // We're showing actual progress
			layerID := progressMsg.Layer.ID
			layerProgress[layerID] = progressMsg.Layer.Current

			// Sum all layer progress
			current = uint64(0)
			for _, layerCurrent := range layerProgress {
				current += layerCurrent
			}

			printer.Println(fmt.Sprintf("Downloaded %s of %s",
				units.CustomSize("%.2f%s", float64(current), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}),
				units.CustomSize("%.2f%s", float64(progressMsg.Total), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"})))

		case oci.TypeSuccess:
			finalMessage = progressMsg.Message

		case oci.TypeWarning:
			// Print warning to stderr
			printer.PrintErrf("Warning: %s\n", progressMsg.Message)

		case oci.TypeError:
			return "", false, fmt.Errorf("%s", progressMsg.Message)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, err
	}

	// Surface unparseable content if no valid progress was received.
	if finalMessage == "" && !progressShown {
		if err := unexpectedProgressDataError(nonJSONBytes, nonJSONTruncated); err != nil {
			return "", false, err
		}
	}

	return finalMessage, progressShown, nil
}

// Status strings used in progress display. All are padded to
// progressStatusWidth so that progress bars line up at the same column.
const (
	progressStatusWaiting      = "Waiting"
	progressStatusDownloading  = "Downloading"
	progressStatusPullComplete = "Pull complete"
	progressStatusUploading    = "Uploading"
	progressStatusPushComplete = "Push complete"

	// progressStatusWidth is the column width to which all status strings
	// are left-padded, keeping progress bars horizontally aligned.
	progressStatusWidth = max(
		len(progressStatusWaiting),
		len(progressStatusDownloading),
		len(progressStatusPullComplete),
		len(progressStatusUploading),
		len(progressStatusPushComplete),
	)
)

// writeDockerProgress writes a progress update in Docker's JSONMessage format
func writeDockerProgress(w io.Writer, msg *oci.ProgressMessage) error {
	layerID := msg.Layer.ID
	if layerID == "" {
		return nil
	}

	// Detect if this is a push operation.
	isPush := msg.Mode == oci.ModePush

	// Determine status based on progress.
	var status string
	var progressDetail *jsonstream.Progress

	if msg.Layer.Current == 0 {
		status = progressStatusWaiting
	} else if msg.Layer.Current < msg.Layer.Size {
		if isPush {
			status = progressStatusUploading
		} else {
			status = progressStatusDownloading
		}
		progressDetail = &jsonstream.Progress{
			Current: int64(msg.Layer.Current),
			Total:   int64(msg.Layer.Size),
		}
	} else if msg.Layer.Current >= msg.Layer.Size && msg.Layer.Size > 0 {
		if isPush {
			status = progressStatusPushComplete
		} else {
			status = progressStatusPullComplete
		}
		progressDetail = &jsonstream.Progress{
			Current: int64(msg.Layer.Current),
			Total:   int64(msg.Layer.Size),
		}
	}

	if status == "" {
		return nil
	}

	// Shorten layer ID for display (similar to Docker).
	displayID := strings.TrimPrefix(layerID, "sha256:")
	if len(displayID) > 12 {
		displayID = displayID[:12]
	}

	dockerMsg := jsonstream.Message{
		ID: displayID,
		// Pad status to a fixed width so all progress bars start at the
		// same column regardless of status string length.
		Status:   fmt.Sprintf("%-*s", progressStatusWidth, status),
		Progress: progressDetail,
	}

	data, err := json.Marshal(dockerMsg)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// writerAdapter adapts StatusPrinter to io.Writer for jsonmessage
type writerAdapter struct {
	printer standalone.StatusPrinter
}

func (w *writerAdapter) Write(p []byte) (n int, err error) {
	return w.printer.Write(p)
}

// simplePrinter is a simple StatusPrinter that just writes to a function
type simplePrinter struct {
	printFunc func(string)
}

func (p *simplePrinter) Printf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	p.printFunc(s)
}

func (p *simplePrinter) Println(args ...any) {
	s := fmt.Sprintln(args...)
	p.printFunc(s)
}

func (p *simplePrinter) PrintErrf(format string, args ...any) {
	// For simple printer, just print to the same output
	s := fmt.Sprintf(format, args...)
	p.printFunc(s)
}

func (p *simplePrinter) Write(p2 []byte) (n int, err error) {
	p.printFunc(string(p2))
	return len(p2), nil
}

func (p *simplePrinter) GetFdInfo() (uintptr, bool) {
	return 0, false
}

// NewSimplePrinter creates a StatusPrinter from a simple print function
func NewSimplePrinter(printFunc func(string)) standalone.StatusPrinter {
	return &simplePrinter{
		printFunc: printFunc,
	}
}

// maxNonJSONBytes is the maximum number of bytes collected from unparseable
// non-JSON lines in the progress stream before truncation.
const maxNonJSONBytes = 4096

// appendNonJSONLine appends line (with a newline separator) to dst, enforcing
// a hard cap of maxNonJSONBytes total. Returns the updated slice and a boolean
// indicating whether the line was truncated to fit within the cap.
func appendNonJSONLine(dst []byte, line string) ([]byte, bool) {
	if len(dst) >= maxNonJSONBytes {
		return dst, true
	}
	if len(dst) > 0 {
		dst = append(dst, '\n')
	}
	remaining := maxNonJSONBytes - len(dst)
	truncated := len(line) > remaining
	if truncated {
		line = line[:remaining]
	}
	return append(dst, line...), truncated
}

// unexpectedProgressDataError returns an error describing unexpected non-JSON
// response data, or nil if nonJSONBytes is empty. If truncated is true, a
// marker is appended to indicate the response was cut off.
func unexpectedProgressDataError(nonJSONBytes []byte, truncated bool) error {
	if len(nonJSONBytes) == 0 {
		return nil
	}
	msg := string(nonJSONBytes)
	if truncated {
		msg += "\n...[truncated]"
	}
	return fmt.Errorf(
		"unexpected response from server (not valid progress data): %s",
		msg,
	)
}
