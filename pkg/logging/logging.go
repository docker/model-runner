package logging

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger is the application logger type, backed by slog.
type Logger = *slog.Logger

// ParseLevel parses a log level string into slog.Level.
// Supported values: debug, info, warn, error (case-insensitive).
// Defaults to info if the value is unrecognized.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger creates a new slog.Logger with a text handler at the given level.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// slogWriter is an io.WriteCloser that forwards each line to a slog.Logger.
type slogWriter struct {
	logger *slog.Logger
	pr     *io.PipeReader
	pw     *io.PipeWriter
	done   chan struct{}
}

// NewWriter returns an io.WriteCloser that logs each line written to it
// using the provided slog.Logger at Info level.
func NewWriter(logger *slog.Logger) io.WriteCloser {
	pr, pw := io.Pipe()
	sw := &slogWriter{
		logger: logger,
		pr:     pr,
		pw:     pw,
		done:   make(chan struct{}),
	}
	go sw.scan()
	return sw
}

func (sw *slogWriter) scan() {
	defer close(sw.done)
	scanner := bufio.NewScanner(sw.pr)
	for scanner.Scan() {
		sw.logger.Log(context.Background(), slog.LevelInfo, scanner.Text())
	}
}

func (sw *slogWriter) Write(p []byte) (int, error) {
	return sw.pw.Write(p)
}

func (sw *slogWriter) Close() error {
	err := sw.pw.Close()
	<-sw.done
	return err
}

// BracketWriter wraps an io.Writer and prefixes each complete line
// with a [timestamp] in the format expected by the logs.MergeLogs
// function. Partial writes are buffered until a newline is received.
//
// This is used to write log files in the same format that Docker
// Desktop produces, so the /logs API endpoint can serve and
// merge-sort them correctly.
type BracketWriter struct {
	w   io.Writer
	mu  sync.Mutex
	buf []byte
}

// NewBracketWriter creates a new BracketWriter that wraps w.
func NewBracketWriter(w io.Writer) *BracketWriter {
	return &BracketWriter{w: w}
}

// Write implements io.Writer. It buffers partial input and writes
// each complete line to the underlying writer with a [timestamp]
// prefix.
func (bw *BracketWriter) Write(p []byte) (int, error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	n := len(p)
	bw.buf = append(bw.buf, p...)

	for {
		idx := bytes.IndexByte(bw.buf, '\n')
		if idx < 0 {
			break
		}
		line := bw.buf[:idx]

		ts := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
		if _, err := fmt.Fprintf(bw.w, "[%s] %s\n", ts, line); err != nil {
			return n, err
		}
		// Advance the buffer only after a successful write to
		// avoid losing the line on transient I/O errors.
		bw.buf = append(bw.buf[:0], bw.buf[idx+1:]...)
	}
	return n, nil
}
