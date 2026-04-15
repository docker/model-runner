// Package logs provides shared log file reading, merging, and
// following utilities for the Docker Model Runner service log and
// engine log files.
package logs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// ServiceLogName is the filename of the DMR service log.
const ServiceLogName = "inference.log"

// EngineLogName is the filename of the DMR engine (llama.cpp) log.
const EngineLogName = "inference-llama.cpp-server.log"

// timestampRe matches the timestamp prefix in DMR log lines.
var timestampRe = regexp.MustCompile(
	`\[(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\].*`,
)

// timeFmt is the time format used in DMR log lines.
const timeFmt = "2006-01-02T15:04:05.999999999Z"

// MergeResult holds the file offsets reached after an initial merge
// read. Pass these to Follow so it resumes from exactly where the
// merge left off, with no gap.
type MergeResult struct {
	ServiceOffset int64
	EngineOffset  int64
}

// MergeLogs reads the service log at serviceLogPath and (when
// engineLogPath is non-empty) the engine log at engineLogPath,
// merges them in timestamp order, and writes the result to w.
// It returns the byte offset reached in each file, for use with
// Follow to avoid missing lines written between the read and
// the tail start.
//
// A missing or unreadable engine log is tolerated: only the
// service log is streamed. A missing service log is a hard error.
func MergeLogs(
	w io.Writer,
	serviceLogPath string,
	engineLogPath string,
) (MergeResult, error) {
	sf, err := os.Open(serviceLogPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("open service log: %w", err)
	}
	defer sf.Close()

	sr := newLogReader(sf)

	// Engine log is optional; a missing file is not an error.
	var er *logReader
	if engineLogPath != "" {
		ef, openErr := os.Open(engineLogPath)
		if openErr == nil {
			defer ef.Close()
			er = newLogReader(ef)
		}
	}

	// Prime both readers.
	sr.advance(w)
	if er != nil {
		er.advance(w)
	}

	// Merge-sort: output the line with the earlier (or equal)
	// timestamp, then advance that reader.
	for sr.pending != "" && er != nil && er.pending != "" {
		// When timestamps are equal, prefer the service log (same
		// behaviour as the original printMergedLog).
		if !er.pendingTS.Before(sr.pendingTS) {
			fmt.Fprintln(w, sr.pending)
			sr.advance(w)
		} else {
			fmt.Fprintln(w, er.pending)
			er.advance(w)
		}
	}

	// Drain the remaining service lines.
	for sr.pending != "" {
		fmt.Fprintln(w, sr.pending)
		sr.advance(w)
	}

	// Drain the remaining engine lines.
	if er != nil {
		for er.pending != "" {
			fmt.Fprintln(w, er.pending)
			er.advance(w)
		}
	}

	result := MergeResult{ServiceOffset: sr.offset}
	if er != nil {
		result.EngineOffset = er.offset
	}
	return result, nil
}

// logReader wraps a buffered file reader and tracks how many bytes
// have been returned by ReadString, providing an accurate offset for
// resuming with nxadm/tail.
type logReader struct {
	r         *bufio.Reader
	offset    int64
	eof       bool
	pending   string
	pendingTS time.Time
}

// newLogReader returns a logReader backed by f with a 64 KiB buffer.
func newLogReader(f *os.File) *logReader {
	return &logReader{r: bufio.NewReaderSize(f, 64*1024)}
}

// readLine reads the next line from the underlying reader, updates
// the byte offset, and returns the line text without the trailing
// newline (and carriage return, if present).
func (lr *logReader) readLine() (string, error) {
	line, err := lr.r.ReadString('\n')
	lr.offset += int64(len(line))
	return strings.TrimRight(line, "\r\n"), err
}

// advance reads lines from the file until it finds one with a
// parseable timestamp (stored in lr.pending) or reaches EOF
// (lr.pending = ""). Lines without a parseable timestamp are
// written to w immediately.
func (lr *logReader) advance(w io.Writer) {
	lr.pending = ""
	if lr.eof {
		return
	}
	for {
		line, err := lr.readLine()
		if err != nil {
			lr.eof = true
		}
		if line != "" {
			match := timestampRe.FindStringSubmatch(line)
			if len(match) == 2 {
				ts, parseErr := time.Parse(timeFmt, match[1])
				if parseErr == nil {
					lr.pending = line
					lr.pendingTS = ts
					return
				}
			}
			// No parseable timestamp: print immediately.
			fmt.Fprintln(w, line)
		}
		if lr.eof {
			return
		}
	}
}
