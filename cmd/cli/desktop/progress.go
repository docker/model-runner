package desktop

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-units"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/pkg/distribution/oci"
)

// DisplayProgress displays progress messages from a model pull/push operation
// using Docker-style multi-line progress bars.
// Returns the final message, whether progress was actually shown, and any error.
func DisplayProgress(body io.Reader, printer standalone.StatusPrinter) (string, bool, error) {
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

	// Convert progress messages to JSONMessage format
	scanner := bufio.NewScanner(body)
	var finalMessage string
	progressShown := false // Track if we actually showed any progress bars

	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		var progressMsg oci.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			// If we can't parse, just skip
			continue
		}

		switch progressMsg.Type {
		case oci.TypeProgress:
			progressShown = true // We're showing actual progress
			if err := writeDockerProgress(pw, &progressMsg); err != nil {
				pw.Close()
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
			pw.Close()
			return "", false, fmt.Errorf("%s", progressMsg.Message)
		}
	}

	if err := scanner.Err(); err != nil {
		pw.Close()
		return "", false, err
	}

	pw.Close()

	// Wait for display to finish
	if err := <-errCh; err != nil && !errors.Is(err, io.EOF) {
		return finalMessage, progressShown, err
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

	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		var progressMsg oci.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
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

	return finalMessage, progressShown, nil
}

// writeDockerProgress writes a progress update in Docker's JSONMessage format
func writeDockerProgress(w io.Writer, msg *oci.ProgressMessage) error {
	layerID := msg.Layer.ID
	if layerID == "" {
		return nil
	}

	// Detect if this is a push operation based on the sentinel layer ID
	isPush := msg.Mode == "push"

	// Determine status based on progress
	var status string
	var progressDetail *jsonmessage.JSONProgress

	if msg.Layer.Current == 0 {
		status = "Waiting"
	} else if msg.Layer.Current < msg.Layer.Size {
		if isPush {
			status = "Uploading"
		} else {
			status = "Downloading"
		}
		progressDetail = &jsonmessage.JSONProgress{
			Current: int64(msg.Layer.Current),
			Total:   int64(msg.Layer.Size),
		}
	} else if msg.Layer.Current >= msg.Layer.Size && msg.Layer.Size > 0 {
		if isPush {
			status = "Push complete"
		} else {
			status = "Pull complete"
		}
		progressDetail = &jsonmessage.JSONProgress{
			Current: int64(msg.Layer.Current),
			Total:   int64(msg.Layer.Size),
		}
	}

	if status == "" {
		return nil
	}

	// Shorten layer ID for display (similar to Docker)
	displayID := strings.TrimPrefix(layerID, "sha256:")
	if len(displayID) > 12 {
		displayID = displayID[:12]
	}

	dockerMsg := jsonmessage.JSONMessage{
		ID:       displayID,
		Status:   status,
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
