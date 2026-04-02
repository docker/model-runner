package logging

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// timestampRe matches the bracket-timestamp prefix produced by BracketWriter.
var timestampRe = regexp.MustCompile(
	`^\[(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\] (.*)$`,
)

func TestBracketWriter_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	bw := NewBracketWriter(&buf)

	n, err := bw.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len("hello world\n") {
		t.Fatalf("expected n=%d, got %d", len("hello world\n"), n)
	}

	output := buf.String()
	matches := timestampRe.FindStringSubmatch(strings.TrimRight(output, "\n"))
	if len(matches) != 3 {
		t.Fatalf("output did not match expected format: %q", output)
	}
	if matches[2] != "hello world" {
		t.Errorf("expected message %q, got %q", "hello world", matches[2])
	}
}

func TestBracketWriter_MultipleLines(t *testing.T) {
	var buf bytes.Buffer
	bw := NewBracketWriter(&buf)

	input := "line one\nline two\nline three\n"
	n, err := bw.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("expected n=%d, got %d", len(input), n)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}

	expected := []string{"line one", "line two", "line three"}
	for i, line := range lines {
		matches := timestampRe.FindStringSubmatch(line)
		if len(matches) != 3 {
			t.Errorf("line %d did not match format: %q", i, line)
			continue
		}
		if matches[2] != expected[i] {
			t.Errorf("line %d: expected %q, got %q", i, expected[i], matches[2])
		}
	}
}

func TestBracketWriter_PartialWrites(t *testing.T) {
	var buf bytes.Buffer
	bw := NewBracketWriter(&buf)

	// Write partial line (no newline yet).
	_, err := bw.Write([]byte("partial"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output for partial line, got %q", buf.String())
	}

	// Complete the line.
	_, err = bw.Write([]byte(" line\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches := timestampRe.FindStringSubmatch(strings.TrimRight(buf.String(), "\n"))
	if len(matches) != 3 {
		t.Fatalf("output did not match format: %q", buf.String())
	}
	if matches[2] != "partial line" {
		t.Errorf("expected %q, got %q", "partial line", matches[2])
	}
}

func TestBracketWriter_EmptyLine(t *testing.T) {
	var buf bytes.Buffer
	bw := NewBracketWriter(&buf)

	_, err := bw.Write([]byte("\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimRight(buf.String(), "\n")
	matches := timestampRe.FindStringSubmatch(output)
	if len(matches) != 3 {
		t.Fatalf("output did not match format: %q", buf.String())
	}
	if matches[2] != "" {
		t.Errorf("expected empty message, got %q", matches[2])
	}
}

func TestBracketWriter_NoTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	bw := NewBracketWriter(&buf)

	// Write without trailing newline — should buffer without output.
	_, err := bw.Write([]byte("no newline"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output without newline, got %q", buf.String())
	}
}
