package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// stripANSI removes color codes so tests can match against raw text.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func clean(s string) string { return ansiRE.ReplaceAllString(s, "") }

func writeLog(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func tsLine(ts string, body string) string { return ts + " " + body }

func TestParseLine(t *testing.T) {
	ts, body, ok := parseLine(tsLine("2026-04-26 12:00:00", "hello"))
	if !ok {
		t.Fatal("expected parse ok")
	}
	if body != "hello" {
		t.Errorf("body = %q", body)
	}
	if ts.Year() != 2026 || ts.Hour() != 12 {
		t.Errorf("ts = %v", ts)
	}

	if _, _, ok := parseLine("=== banner ==="); ok {
		t.Error("banner should not parse as ts line")
	}
	if _, _, ok := parseLine("short"); ok {
		t.Error("short string should not parse")
	}
}

func TestReadEntries_Continuation(t *testing.T) {
	r := strings.NewReader(strings.Join([]string{
		"2026-04-26 12:00:00 first line",
		"continuation A",
		"continuation B",
		"2026-04-26 12:00:01 second line",
		"",
	}, "\n"))
	entries, _ := readEntries(r, "STDOUT", 0)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if !strings.Contains(entries[0].body, "continuation A") || !strings.Contains(entries[0].body, "continuation B") {
		t.Errorf("continuations not folded: %q", entries[0].body)
	}
	if entries[1].body != "second line" {
		t.Errorf("second body = %q", entries[1].body)
	}
}

func TestMergeByTS_Chronological(t *testing.T) {
	stdout := []entry{
		{ts: mustTime("2026-04-26 12:00:01"), label: "STDOUT", body: "ok 1", hasTS: true, seq: 0},
		{ts: mustTime("2026-04-26 12:00:03"), label: "STDOUT", body: "ok 2", hasTS: true, seq: 1},
	}
	stderr := []entry{
		{ts: mustTime("2026-04-26 12:00:02"), label: "STDERR", body: "err 1", hasTS: true, seq: 2},
		{ts: mustTime("2026-04-26 12:00:04"), label: "STDERR", body: "err 2", hasTS: true, seq: 3},
	}
	merged := mergeByTS(stdout, stderr)
	want := []string{"ok 1", "err 1", "ok 2", "err 2"}
	if len(merged) != len(want) {
		t.Fatalf("len = %d, want %d", len(merged), len(want))
	}
	for i, w := range want {
		if merged[i].body != w {
			t.Errorf("[%d] = %q, want %q", i, merged[i].body, w)
		}
	}
}

func TestBoundedTail_TakesNewestAcrossSources(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")

	// stdout: 30 lines at 12:00:00..12:00:29
	stdoutLines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		stdoutLines = append(stdoutLines, fmt.Sprintf("2026-04-26 12:00:%02d out-%d", i, i))
	}
	writeLog(t, stdoutPath, stdoutLines...)

	// stderr: 10 lines at 12:00:30..12:00:39 (newer than all stdout)
	stderrLines := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		stderrLines = append(stderrLines, fmt.Sprintf("2026-04-26 12:00:%02d err-%d", 30+i, i))
	}
	writeLog(t, stderrPath, stderrLines...)

	var buf bytes.Buffer
	srcs := []streamSource{
		{path: stdoutPath, label: "STDOUT"},
		{path: stderrPath, label: "STDERR"},
	}
	if err := boundedTail(&buf, srcs, 40, filter{}); err != nil {
		t.Fatalf("boundedTail: %v", err)
	}
	out := clean(buf.String())
	got := strings.Count(out, "out-") + strings.Count(out, "err-")
	if got != 40 {
		t.Errorf("expected 40 entries, got %d\n%s", got, out)
	}

	// Last 10 lines of output should all be err-* (newer)
	tailLines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for _, l := range tailLines[len(tailLines)-10:] {
		if !strings.Contains(l, "err-") {
			t.Errorf("expected newer err entries at tail, got %q", l)
		}
	}
}

func TestBoundedTail_StderrSparseFillsFromStdout(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")

	// stdout: 50 lines (more than tail)
	stdoutLines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		stdoutLines = append(stdoutLines, fmt.Sprintf("2026-04-26 12:00:%02d out-%d", i%60, i))
	}
	writeLog(t, stdoutPath, stdoutLines...)

	// stderr: only 10 lines
	stderrLines := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		stderrLines = append(stderrLines, fmt.Sprintf("2026-04-26 12:01:%02d err-%d", i, i))
	}
	writeLog(t, stderrPath, stderrLines...)

	var buf bytes.Buffer
	srcs := []streamSource{
		{path: stdoutPath, label: "STDOUT"},
		{path: stderrPath, label: "STDERR"},
	}
	if err := boundedTail(&buf, srcs, 40, filter{}); err != nil {
		t.Fatalf("boundedTail: %v", err)
	}
	out := clean(buf.String())
	errCount := strings.Count(out, "err-")
	outCount := strings.Count(out, "out-")
	if errCount != 10 {
		t.Errorf("expected all 10 err entries, got %d", errCount)
	}
	if errCount+outCount != 40 {
		t.Errorf("expected 40 total, got %d (err=%d out=%d)", errCount+outCount, errCount, outCount)
	}
}

func TestStreamMerge_All(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")

	writeLog(t, stdoutPath,
		"2026-04-26 12:00:01 a",
		"2026-04-26 12:00:03 c",
	)
	writeLog(t, stderrPath,
		"2026-04-26 12:00:02 b",
		"2026-04-26 12:00:04 d",
	)

	var buf bytes.Buffer
	srcs := []streamSource{
		{path: stdoutPath, label: "STDOUT"},
		{path: stderrPath, label: "STDERR"},
	}
	if err := streamMerge(context.Background(), &buf, filter{}, srcs...); err != nil {
		t.Fatalf("streamMerge: %v", err)
	}
	out := clean(buf.String())
	want := []string{"a", "b", "c", "d"}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("len = %d, want %d:\n%s", len(lines), len(want), out)
	}
	for i, w := range want {
		if !strings.HasSuffix(lines[i], " "+w) {
			t.Errorf("[%d] = %q, want suffix %q", i, lines[i], w)
		}
	}
}

func TestStreamMerge_FoldsContinuation(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	writeLog(t, stdoutPath,
		"2026-04-26 12:00:01 first",
		"trace-A",
		"trace-B",
		"2026-04-26 12:00:02 second",
	)
	var buf bytes.Buffer
	if err := streamMerge(context.Background(), &buf, filter{},
		streamSource{path: stdoutPath, label: "STDOUT"}); err != nil {
		t.Fatalf("streamMerge: %v", err)
	}
	out := clean(buf.String())
	if !strings.Contains(out, "first\ntrace-A\ntrace-B") {
		t.Errorf("continuation not folded:\n%s", out)
	}
	if !strings.Contains(out, "second") {
		t.Errorf("second entry missing:\n%s", out)
	}
}

func TestFilter_Since(t *testing.T) {
	now := mustTime("2026-04-26 12:00:00")
	fs := filter{since: now}

	old := entry{ts: mustTime("2026-04-26 11:59:59"), hasTS: true, body: "old"}
	cur := entry{ts: mustTime("2026-04-26 12:00:30"), hasTS: true, body: "cur"}
	if fs.keep(old) {
		t.Error("old should be filtered")
	}
	if !fs.keep(cur) {
		t.Error("cur should pass")
	}
}

func TestFilter_Grep(t *testing.T) {
	re := regexp.MustCompile(`(?i)error`)
	fs := filter{grep: re}

	if fs.keep(entry{body: "ok"}) {
		t.Error("non-match kept")
	}
	if !fs.keep(entry{body: "fatal ERROR here"}) {
		t.Error("match dropped")
	}
}

func TestGuard_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.log")
	writeLog(t, p, "2026-04-26 12:00:00 small")
	srcs := []streamSource{{path: p, label: "STDOUT"}}
	if err := guardLargeRead(srcs, false, strings.NewReader("")); err != nil {
		t.Errorf("expected no guard for small file, got %v", err)
	}
}

func TestGuard_BlockWithoutYes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.log")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(blockSizeBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	srcs := []streamSource{{path: p, label: "STDOUT"}}
	err = guardLargeRead(srcs, false, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected block error, got %v", err)
	}

	// With --yes the guard lets it through.
	if err := guardLargeRead(srcs, true, strings.NewReader("")); err != nil {
		t.Errorf("--yes should bypass block, got %v", err)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{500, "500 B"},
		{2 * 1024, "2.0 KiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
		{3 * 1024 * 1024 * 1024, "3.0 GiB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.n); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		check  func(t *testing.T, o options)
		errMsg string
	}{
		{
			name: "defaults",
			args: []string{"api"},
			check: func(t *testing.T, o options) {
				if o.lines != 40 || o.follow || o.all || !o.showStdout || !o.showStderr {
					t.Errorf("bad defaults: %+v", o)
				}
			},
		},
		{
			name: "tail flag",
			args: []string{"api", "--tail", "100"},
			check: func(t *testing.T, o options) {
				if o.lines != 100 {
					t.Errorf("lines = %d", o.lines)
				}
			},
		},
		{
			name: "since",
			args: []string{"api", "--since", "1h"},
			check: func(t *testing.T, o options) {
				if o.since != time.Hour {
					t.Errorf("since = %v", o.since)
				}
			},
		},
		{
			name:   "bad since",
			args:   []string{"api", "--since", "invalid"},
			errMsg: "invalid --since",
		},
		{
			name:   "missing target",
			args:   []string{"-f"},
			errMsg: "missing process",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := parseArgs(c.args)
			if c.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), c.errMsg) {
					t.Errorf("err = %v, want contains %q", err, c.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			c.check(t, opts)
		})
	}
}

// readLastN smoke check on a real-sized file.
func TestReadLastNEntries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.log")
	lines := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("2026-04-26 12:00:%02d line-%d", i%60, i))
	}
	writeLog(t, p, lines...)

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	entries, _, err := readLastNEntries(f, "STDOUT", 30, 0)
	if err != nil {
		t.Fatalf("readLastNEntries: %v", err)
	}
	if len(entries) != 30 {
		t.Errorf("got %d entries, want 30", len(entries))
	}
	if !strings.HasSuffix(entries[len(entries)-1].body, "line-199") {
		t.Errorf("last body = %q, want suffix line-199", entries[len(entries)-1].body)
	}
}

func mustTime(s string) time.Time {
	t, err := time.ParseInLocation(tsLayout, s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// io.Discard sanity (silences unused import warnings if test setup
// changes during refactor).
var _ = io.Discard
