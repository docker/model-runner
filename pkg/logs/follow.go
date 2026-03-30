package logs

import (
	"context"
	"fmt"
	"io"

	"github.com/nxadm/tail"
)

// Follow tails the service log (and optionally the engine log) from
// the offsets recorded by a prior MergeLogs call, writing new lines
// to w until ctx is cancelled.
//
// Using the recorded offsets avoids the gap that would occur if
// tailing started from the end-of-file: any lines written between
// the end of MergeLogs and the start of Follow are included.
//
// A single select loop serialises writes from both tail channels so
// that w does not need to be concurrency-safe (important for
// http.ResponseWriter).
//
// pollMode should be true on Windows and when accessing files
// through a mounted filesystem (e.g. WSL2 accessing Windows paths),
// where filesystem event notifications are unreliable.
func Follow(
	ctx context.Context,
	w io.Writer,
	serviceLogPath string,
	engineLogPath string,
	offsets MergeResult,
	pollMode bool,
) error {
	makeCfg := func(offset int64) tail.Config {
		return tail.Config{
			Location: &tail.SeekInfo{
				Offset: offset,
				Whence: io.SeekStart,
			},
			Follow: true,
			ReOpen: true,
			Poll:   pollMode,
			Logger: tail.DiscardingLogger,
		}
	}

	st, err := tail.TailFile(serviceLogPath, makeCfg(offsets.ServiceOffset))
	if err != nil {
		return fmt.Errorf("tail service log: %w", err)
	}
	defer st.Cleanup()
	defer st.Stop() //nolint:errcheck

	// Engine log is optional; a missing file is tolerated.
	var et *tail.Tail
	if engineLogPath != "" {
		et, err = tail.TailFile(engineLogPath, makeCfg(offsets.EngineOffset))
		if err != nil {
			et = nil
		} else {
			defer et.Cleanup()
			defer func(et *tail.Tail) {
				_ = et.Stop()
			}(et)
		}
	}

	return followLoop(ctx, w, st, et)
}

// followLoop runs the single-writer select loop for up to two tail
// channels, returning nil when ctx is cancelled or an error if a
// tail channel closes unexpectedly.
func followLoop(
	ctx context.Context,
	w io.Writer,
	st *tail.Tail,
	et *tail.Tail,
) error {
	// When there is no engine tail, use a nil channel that blocks
	// forever so the select always waits on st.Lines and ctx.Done().
	var etLines <-chan *tail.Line
	if et != nil {
		etLines = et.Lines
	}

	for {
		select {
		case line, ok := <-st.Lines:
			if !ok {
				return st.Err()
			}
			fmt.Fprintln(w, line.Text)
		case line, ok := <-etLines:
			if !ok {
				return et.Err()
			}
			fmt.Fprintln(w, line.Text)
		case <-ctx.Done():
			return nil
		}
	}
}
