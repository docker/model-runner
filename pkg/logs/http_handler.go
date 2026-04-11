package logs

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// NewHTTPHandler returns an HTTP handler that streams DMR log files
// from logDir. Only the fixed filenames ServiceLogName and
// EngineLogName are served; arbitrary paths are not accepted.
//
// Query parameters:
//   - follow (bool): if true, tail the files after the initial read.
//   - no-engines (bool): if true, exclude the engine log.
//
// The handler returns:
//   - 400 if a query parameter cannot be parsed as a boolean.
//   - 404 if the service log file does not exist.
//   - 501 if logDir is empty (logs API not configured).
//   - 200 with Content-Type text/plain on success.
func NewHTTPHandler(logDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if logDir == "" {
			http.Error(
				w,
				"logs API not available: log directory not configured",
				http.StatusNotImplemented,
			)
			return
		}

		// Parse query parameters.
		follow, err := parseBoolParam(r, "follow")
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid follow parameter: %v", err), http.StatusBadRequest)
			return
		}
		noEngines, err := parseBoolParam(r, "no-engines")
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid no-engines parameter: %v", err), http.StatusBadRequest)
			return
		}

		serviceLogPath := filepath.Join(logDir, ServiceLogName)
		var engineLogPath string
		if !noEngines {
			engineLogPath = filepath.Join(logDir, EngineLogName)
		}

		// Verify the service log exists before starting to write
		// the response body.
		if _, err := os.Stat(serviceLogPath); os.IsNotExist(err) {
			http.Error(w, "service log not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain")

		// Stream the initial merged content.
		result, err := MergeLogs(w, serviceLogPath, engineLogPath)
		if err != nil {
			// Headers already sent; best effort to write the error.
			fmt.Fprintf(w, "\n[logs error: %v]\n", err)
			return
		}

		if !follow {
			return
		}

		// Flush the initial content before starting the tail.
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Follow: tail from recorded offsets using a flushing writer.
		// pollMode is needed on Windows where filesystem event
		// notifications are unreliable.
		pollMode := runtime.GOOS == "windows"
		var out io.Writer = w
		if flusher, ok := w.(http.Flusher); ok {
			out = &flushingWriter{w: w, flusher: flusher}
		}
		if err := Follow(
			r.Context(), out,
			serviceLogPath, engineLogPath,
			result, pollMode,
		); err != nil {
			fmt.Fprintf(w, "\n[logs error: %v]\n", err)
		}
	}
}

// parseBoolParam reads a single query parameter as a bool.
// Returns false and nil error when the parameter is absent.
// Returns an error when the parameter is present but unparseable.
func parseBoolParam(r *http.Request, name string) (bool, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("%q is not a valid boolean", s)
	}
	return v, nil
}

// flushingWriter wraps an io.Writer and flushes after each Write
// call, ensuring that lines reach the HTTP client without buffering.
type flushingWriter struct {
	w       io.Writer
	flusher interface{ Flush() }
}

// Write writes p to the underlying writer and flushes immediately.
func (fw *flushingWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err == nil {
		fw.flusher.Flush()
	}
	return n, err
}
