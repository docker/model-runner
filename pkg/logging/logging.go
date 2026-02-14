package logging

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
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
