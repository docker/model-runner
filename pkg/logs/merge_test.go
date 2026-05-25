package logs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMergeLogsEqualTimestamps verifies that MergeLogs does not
// deadlock when both files contain lines with the same timestamp.
func TestMergeLogsEqualTimestamps(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(f1, []byte("[2026-03-09T12:00:00.000000000Z] line a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(t.TempDir(), "b.log")
	if err := os.WriteFile(f2, []byte("[2026-03-09T12:00:00.000000000Z] line b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := MergeLogs(io.Discard, f1, f2); done <- err }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("MergeLogs returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("MergeLogs hung — possible deadlock on equal timestamps")
	}
}

// TestMergeLogsInterleavedTimestamps verifies that lines from two
// files are interleaved in ascending timestamp order.
func TestMergeLogsInterleavedTimestamps(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(f1, []byte(strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] a1",
		"[2026-03-09T12:00:02.000000000Z] a2",
		"[2026-03-09T12:00:04.000000000Z] a3",
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(t.TempDir(), "b.log")
	if err := os.WriteFile(f2, []byte(strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] b1",
		"[2026-03-09T12:00:01.000000000Z] b2",
		"[2026-03-09T12:00:03.000000000Z] b3",
		"[2026-03-09T12:00:05.000000000Z] b4",
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_, err := MergeLogs(&buf, f1, f2)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] a1",
		"[2026-03-09T12:00:00.000000000Z] b1",
		"[2026-03-09T12:00:01.000000000Z] b2",
		"[2026-03-09T12:00:02.000000000Z] a2",
		"[2026-03-09T12:00:03.000000000Z] b3",
		"[2026-03-09T12:00:04.000000000Z] a3",
		"[2026-03-09T12:00:05.000000000Z] b4",
	}, "\n")

	if got != want {
		t.Errorf("unexpected merge order:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestMergeLogsServiceOnly verifies that an empty engineLogPath
// causes only the service log to be streamed.
func TestMergeLogsServiceOnly(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "service.log")
	content := "[2026-03-09T12:00:00.000000000Z] service line\n"
	if err := os.WriteFile(f1, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	result, err := MergeLogs(&buf, f1, "")
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := strings.TrimSpace(content)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Offset should equal the file size.
	info, _ := os.Stat(f1)
	if result.ServiceOffset != info.Size() {
		t.Errorf("ServiceOffset = %d, want %d", result.ServiceOffset, info.Size())
	}
	if result.EngineOffset != 0 {
		t.Errorf("EngineOffset = %d, want 0", result.EngineOffset)
	}
}

// TestMergeLogsMissingEngineFile verifies that a missing engine log
// is silently tolerated.
func TestMergeLogsMissingEngineFile(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "service.log")
	if err := os.WriteFile(f1, []byte("[2026-03-09T12:00:00.000000000Z] svc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_, err := MergeLogs(&buf, f1, filepath.Join(dir, "nonexistent.log"))
	if err != nil {
		t.Fatalf("expected no error for missing engine log, got: %v", err)
	}
	if !strings.Contains(buf.String(), "svc") {
		t.Errorf("expected service content in output, got: %q", buf.String())
	}
}

// TestMergeLogsMissingServiceFile verifies that a missing service
// log returns an error.
func TestMergeLogsMissingServiceFile(t *testing.T) {
	_, err := MergeLogs(io.Discard, "/nonexistent/path/service.log", "")
	if err == nil {
		t.Error("expected error for missing service log, got nil")
	}
}

// TestMergeLogsNonTimestampedLines verifies that lines without
// timestamps are printed immediately rather than dropped.
func TestMergeLogsNonTimestampedLines(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "service.log")
	content := "no timestamp here\n[2026-03-09T12:00:00.000000000Z] ts line\n"
	if err := os.WriteFile(f1, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_, err := MergeLogs(&buf, f1, "")
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "no timestamp here") {
		t.Errorf("non-timestamped line missing from output: %q", got)
	}
	if !strings.Contains(got, "ts line") {
		t.Errorf("timestamped line missing from output: %q", got)
	}
}

// TestMergeLogsOffsets verifies that MergeResult offsets match
// the total bytes read from each file.
func TestMergeLogsOffsets(t *testing.T) {
	dir := t.TempDir()
	svc := "[2026-03-09T12:00:00.000000000Z] svc\n"
	eng := "[2026-03-09T12:00:01.000000000Z] eng\n"

	sf := filepath.Join(dir, "service.log")
	ef := filepath.Join(dir, "engine.log")
	if err := os.WriteFile(sf, []byte(svc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ef, []byte(eng), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := MergeLogs(io.Discard, sf, ef)
	if err != nil {
		t.Fatal(err)
	}

	sfInfo, _ := os.Stat(sf)
	efInfo, _ := os.Stat(ef)
	if result.ServiceOffset != sfInfo.Size() {
		t.Errorf("ServiceOffset = %d, want %d", result.ServiceOffset, sfInfo.Size())
	}
	if result.EngineOffset != efInfo.Size() {
		t.Errorf("EngineOffset = %d, want %d", result.EngineOffset, efInfo.Size())
	}
}
