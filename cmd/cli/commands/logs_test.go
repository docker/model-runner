package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMergedLogEqualTimestamps(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(f1, []byte("[2026-03-09T12:00:00.000000000Z] line a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(t.TempDir(), "b.log")
	if err := os.WriteFile(f2, []byte("[2026-03-09T12:00:00.000000000Z] line b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- printMergedLog(io.Discard, f1, f2) }()

	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("printMergedLog hung — equal timestamp deadlock")
	}
}

func TestMergedLogInterleavedTimestamps(t *testing.T) {
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
	err := printMergedLog(&buf, f1, f2)
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
		t.Errorf("wrong merge order:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
