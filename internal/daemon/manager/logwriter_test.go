package manager

import (
	"bytes"
	"os"
	"path/filepath"
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

// TestRotatingTimestampWriter_MaybeRotate verifies the writer's rotation
// path: when the underlying file has grown past LYNX_LOG_MAX_BYTES, a call
// to maybeRotate compresses the current file to .1.gz and truncates it.
// This is the same code path the periodic ticker drives in production.
func TestRotatingTimestampWriter_MaybeRotate(t *testing.T) {
	t.Setenv("LYNX_LOG_MAX_BYTES", "100")
	t.Setenv("LYNX_LOG_KEEP", "3")

	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")

	// Seed the file above the threshold before opening with O_APPEND.
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 500), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	tw := newRotatingTimestampWriter(f, path)
	tw.maybeRotate()

	if _, err := os.Stat(path + ".1.gz"); err != nil {
		t.Fatalf("expected %s.1.gz: %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("current log not truncated, size=%d", info.Size())
	}
}

// TestRotatingTimestampWriter_NoRotateBelowThreshold pins down the negative
// case: if size < threshold, maybeRotate is a no-op.
func TestRotatingTimestampWriter_NoRotateBelowThreshold(t *testing.T) {
	t.Setenv("LYNX_LOG_MAX_BYTES", "1000000")

	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(path, []byte("small"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	tw := newRotatingTimestampWriter(f, path)
	tw.maybeRotate()

	if _, err := os.Stat(path + ".1.gz"); !os.IsNotExist(err) {
		t.Errorf("did not expect rotation, but %s.1.gz exists (err=%v)", path, err)
	}
}

// TestRotatingTimestampWriter_DisabledWithEmptyPath ensures the
// non-rotating constructor (used by unit tests that wrap a bytes.Buffer)
// never tries to stat or rotate. Regression guard for accidentally
// enabling rotation on the test path.
func TestRotatingTimestampWriter_DisabledWithEmptyPath(t *testing.T) {
	var buf bytes.Buffer
	tw := newTimestampWriter(&buf)

	// Force a rotation attempt — should be a silent no-op since path == "".
	tw.maybeRotate()
	if _, err := tw.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.HasSuffix(buf.String(), " hello\n") {
		t.Errorf("write path should still work: %q", buf.String())
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
