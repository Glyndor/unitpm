package manager

import (
	"bytes"
	"strings"
	"testing"
)

func TestTimestampWriter_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	_, err := tw.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.HasSuffix(out, " hello world\n") {
		t.Errorf("unexpected output: %q", out)
	}
	// Timestamp: "2006-01-02 15:04:05 " = 20 chars
	if len(out) != 20+len("hello world\n") {
		t.Errorf("unexpected length: %d", len(out))
	}
}

func TestTimestampWriter_MultipleLines(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	_, _ = tw.Write([]byte("line1\nline2\nline3\n"))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	for i, line := range lines {
		suffix := "line" + string(rune('1'+i))
		if !strings.HasSuffix(line, " "+suffix) {
			t.Errorf("line %d: expected suffix %q, got %q", i, suffix, line)
		}
	}
}

func TestTimestampWriter_PartialLines(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	_, _ = tw.Write([]byte("hel"))
	if buf.Len() != 0 {
		t.Error("partial line should not flush")
	}

	_, _ = tw.Write([]byte("lo\n"))
	if !strings.HasSuffix(buf.String(), " hello\n") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestTimestampWriter_BatchSingleWrite(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	_, _ = tw.Write([]byte("a\nb\n"))

	// Should produce exactly 2 writes to underlying writer (batched)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestTimestampWriter_LargeBufferFlush(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	// Write >1MB without newline
	big := strings.Repeat("x", (1<<20)+1)
	_, _ = tw.Write([]byte(big))

	out := buf.String()
	if out == "" {
		t.Error("expected flush on >1MB buffer")
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("flushed buffer should end with newline")
	}
}

func TestTimestampWriter_EmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	n, err := tw.Write([]byte{})
	if err != nil || n != 0 {
		t.Errorf("empty write: n=%d err=%v", n, err)
	}
	if buf.Len() != 0 {
		t.Error("empty write should produce no output")
	}
}

func TestWriteBanner_Format(t *testing.T) {
	var buf bytes.Buffer
	writeBanner(&buf, "STARTED", "")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		if i == 1 {
			continue
		}
		if line != strings.Repeat("==", bannerWidth/2) {
			t.Errorf("line %d not full sep: %q", i, line)
		}
	}
	if !strings.Contains(lines[1], "STARTED") {
		t.Errorf("middle missing event: %q", lines[1])
	}
	if !strings.HasSuffix(lines[1], "==") {
		t.Errorf("middle should end with ==: %q", lines[1])
	}
	if len(lines[1]) != bannerWidth {
		t.Errorf("middle width = %d, want %d: %q", len(lines[1]), bannerWidth, lines[1])
	}
}

func TestWriteBanner_WithDetail(t *testing.T) {
	var buf bytes.Buffer
	writeBanner(&buf, "AUTO-RESTART", "attempt=3 delay=4s")

	out := buf.String()
	if !strings.Contains(out, "AUTO-RESTART") || !strings.Contains(out, "attempt=3 delay=4s") {
		t.Errorf("missing event/detail: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if len(lines[1]) < bannerWidth {
		t.Errorf("middle width %d below min %d: %q", len(lines[1]), bannerWidth, lines[1])
	}
}
