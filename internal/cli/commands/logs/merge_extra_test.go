package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// --- parseLine edge cases -----------------------------------------------

func TestParseLine_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"too short", "2026-04-26", false},
		{"bad date", "2026-99-99 99:99:99 oops", false},
		{"exactly ts no body", "2026-04-26 12:00:00 ", true},
		{"valid trailing space", "2026-04-26 12:00:00  body", true},
		{"empty", "", false},
		{"banner equals", "================================", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, ok := parseLine(c.in)
			if ok != c.ok {
				t.Errorf("parseLine(%q) ok=%v, want %v", c.in, ok, c.ok)
			}
		})
	}
}

// --- readLastNEntries edge cases ----------------------------------------

func TestReadLastNEntries_TinyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.log")
	writeLog(t, p,
		"2026-04-26 12:00:00 a",
		"2026-04-26 12:00:01 b",
	)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	entries, _, err := readLastNEntries(f, "STDOUT", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d, want 2", len(entries))
	}
}

func TestReadLastNEntries_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	entries, _, err := readLastNEntries(f, "STDOUT", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadLastNEntries_LongLineForcesExpansion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "long.log")
	// Build a file where each entry is ~5KB so the n*200 heuristic
	// underestimates and the loop has to widen the window.
	long := strings.Repeat("x", 5000)
	lines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("2026-04-26 12:00:%02d %s-%d", i%60, long, i))
	}
	writeLog(t, p, lines...)

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	entries, _, err := readLastNEntries(f, "STDOUT", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Errorf("got %d entries, want 10", len(entries))
	}
	if !strings.HasSuffix(entries[len(entries)-1].body, "-49") {
		t.Errorf("last suffix mismatch: %q", entries[len(entries)-1].body[:50])
	}
}

// --- mergeByTS edge cases -----------------------------------------------

func TestMergeByTS_Empty(t *testing.T) {
	got := mergeByTS()
	if len(got) != 0 {
		t.Errorf("empty input → %d entries", len(got))
	}
}

func TestMergeByTS_SingleSource(t *testing.T) {
	src := []entry{
		{ts: mustTime("2026-04-26 12:00:01"), body: "a", hasTS: true, seq: 0},
		{ts: mustTime("2026-04-26 12:00:02"), body: "b", hasTS: true, seq: 1},
	}
	got := mergeByTS(src)
	if len(got) != 2 || got[0].body != "a" || got[1].body != "b" {
		t.Errorf("single-source merge: %+v", got)
	}
}

func TestMergeByTS_TieBreakBySeq(t *testing.T) {
	a := []entry{{ts: mustTime("2026-04-26 12:00:00"), body: "a", hasTS: true, seq: 0}}
	b := []entry{{ts: mustTime("2026-04-26 12:00:00"), body: "b", hasTS: true, seq: 1}}
	got := mergeByTS(a, b)
	if got[0].body != "a" || got[1].body != "b" {
		t.Errorf("tie-break order wrong: %+v", got)
	}
}

// --- streamMerge missing source ----------------------------------------

func TestStreamMerge_OneSourceMissing(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "absent.log") // never created
	writeLog(t, stdoutPath, "2026-04-26 12:00:00 only")

	var buf bytes.Buffer
	err := streamMerge(context.Background(), &buf, filter{},
		streamSource{path: stdoutPath, label: "STDOUT"},
		streamSource{path: stderrPath, label: "STDERR"},
	)
	if err != nil {
		t.Fatalf("streamMerge: %v", err)
	}
	out := clean(buf.String())
	if !strings.Contains(out, "only") {
		t.Errorf("missing entry from existing source: %q", out)
	}
}

// --- boundedTail missing all -------------------------------------------

func TestBoundedTail_AllMissing(t *testing.T) {
	dir := t.TempDir()
	srcs := []streamSource{
		{path: filepath.Join(dir, "no1.log"), label: "STDOUT"},
		{path: filepath.Join(dir, "no2.log"), label: "STDERR"},
	}
	var buf bytes.Buffer
	if err := boundedTail(&buf, srcs, 10, filter{}); err != nil {
		t.Errorf("expected nil err for all-missing, got %v", err)
	}
}

// --- buildSources dedup ------------------------------------------------

func TestBuildSources_DedupsSamePath(t *testing.T) {
	// When stdout and stderr resolve to the same absolute path,
	// buildSources must drop the stderr entry to avoid double-emitting
	// every line during the merge.
	dir := t.TempDir()
	app := &protocol.AppSpec{
		ID:   "dedup-test-id",
		Name: "dedup",
		Logs: &protocol.AppLogs{Mode: "file", Dir: dir, Stdout: "shared.log", Stderr: "shared.log"},
	}
	srcs, err := buildSources(app, options{showStdout: true, showStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 {
		t.Errorf("expected 1 source after dedup, got %d (%+v)", len(srcs), srcs)
	}
}

func TestBuildSources_StdoutOnly(t *testing.T) {
	dir := t.TempDir()
	app := &protocol.AppSpec{
		ID:   "stdout-only-id",
		Logs: &protocol.AppLogs{Mode: "file", Dir: dir, Stdout: "a.log", Stderr: "b.log"},
	}
	srcs, err := buildSources(app, options{showStdout: true, showStderr: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 || srcs[0].label != "STDOUT" {
		t.Errorf("expected only STDOUT, got %+v", srcs)
	}
}

// --- buildFilter bad regex ---------------------------------------------

func TestBuildFilter_BadRegex(t *testing.T) {
	_, err := buildFilter(options{grep: "(["})
	if err == nil {
		t.Fatal("expected regex compile error")
	}
}

func TestBuildFilter_SinceClock(t *testing.T) {
	fs, err := buildFilter(options{since: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if fs.since.IsZero() {
		t.Error("since cutoff should be non-zero")
	}
	if time.Since(fs.since) < 59*time.Minute {
		t.Errorf("since cutoff too recent: %v", fs.since)
	}
}

// --- guard branches ----------------------------------------------------

func makeFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if size > 0 {
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	return p
}

func TestGuard_WarnRange_YesSkipsPrompt(t *testing.T) {
	dir := t.TempDir()
	p := makeFile(t, dir, "mid.log", warnSizeBytes+1)
	srcs := []streamSource{{path: p, label: "STDOUT"}}
	if err := guardLargeRead(srcs, true, strings.NewReader("")); err != nil {
		t.Errorf("--yes should bypass warn, got %v", err)
	}
}

func TestGuard_WarnRange_NonTTYProceeds(t *testing.T) {
	dir := t.TempDir()
	p := makeFile(t, dir, "mid.log", warnSizeBytes+1)
	srcs := []streamSource{{path: p, label: "STDOUT"}}
	// In `go test` stdout is a pipe → IsTTY() returns false → guard
	// emits a warning and proceeds without prompting.
	if err := guardLargeRead(srcs, false, strings.NewReader("")); err != nil {
		t.Errorf("non-TTY should proceed, got %v", err)
	}
}

func TestGuard_BlockMissingFiles(t *testing.T) {
	srcs := []streamSource{
		{path: "/nonexistent/path-1.log", label: "STDOUT"},
		{path: "/nonexistent/path-2.log", label: "STDERR"},
	}
	// Missing files contribute 0 bytes → guard should pass quietly.
	if err := guardLargeRead(srcs, false, strings.NewReader("")); err != nil {
		t.Errorf("missing files should not trigger guard, got %v", err)
	}
}

func TestFormatBytes_ZeroAndKiB(t *testing.T) {
	if got := formatBytes(0); got != "0 B" {
		t.Errorf("formatBytes(0) = %q", got)
	}
	if got := formatBytes(1023); got != "1023 B" {
		t.Errorf("formatBytes(1023) = %q", got)
	}
}

// --- parseArgs additional flags ----------------------------------------

func TestParseArgs_AllFlags(t *testing.T) {
	opts, err := parseArgs([]string{
		"api",
		"--all", "--yes",
		"--grep", "ERROR",
		"--stderr",
		"--no-merge",
		"-n", "200",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.all || !opts.yes || opts.grep != "ERROR" || !opts.noMerge || opts.lines != 200 {
		t.Errorf("flags not applied: %+v", opts)
	}
	if !opts.showStderr || opts.showStdout {
		t.Errorf("stderr-only filter wrong: %+v", opts)
	}
}

func TestParseArgs_ShortGrep(t *testing.T) {
	opts, err := parseArgs([]string{"api", "-g", "panic"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.grep != "panic" {
		t.Errorf("grep = %q", opts.grep)
	}
}

// --- entryHeap (used by followMerge) -----------------------------------

func TestEntryHeap_OrdersByTS(t *testing.T) {
	h := &entryHeap{}
	h.Push(entry{ts: mustTime("2026-04-26 12:00:03"), body: "c", seq: 2})
	h.Push(entry{ts: mustTime("2026-04-26 12:00:01"), body: "a", seq: 0})
	h.Push(entry{ts: mustTime("2026-04-26 12:00:02"), body: "b", seq: 1})

	// Direct slice access, since we call Push but not heap.Init/Pop:
	// reorder via sort to validate Less ordering deterministically.
	got := make([]entry, 0, h.Len())
	for h.Len() > 0 {
		// pop minimum manually using Less
		minIdx := 0
		for i := 1; i < h.Len(); i++ {
			if h.Less(i, minIdx) {
				minIdx = i
			}
		}
		got = append(got, (*h)[minIdx])
		h.Swap(minIdx, h.Len()-1)
		_ = h.Pop()
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if got[i].body != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].body, w)
		}
	}
}

func TestEntryHeap_TieBreakBySeq(t *testing.T) {
	h := &entryHeap{}
	ts := mustTime("2026-04-26 12:00:00")
	h.Push(entry{ts: ts, body: "second", seq: 5})
	h.Push(entry{ts: ts, body: "first", seq: 3})
	if !h.Less(1, 0) {
		t.Errorf("expected seq=3 to sort before seq=5")
	}
}

func TestEntryHeap_PushNonEntry(t *testing.T) {
	h := &entryHeap{}
	h.Push("not an entry") // silently dropped
	if h.Len() != 0 {
		t.Errorf("non-entry should be ignored, len=%d", h.Len())
	}
}

// --- waitOpen + tailFollow happy path ----------------------------------

func TestWaitOpen_FileAppearsLater(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "delayed.log")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(p, []byte("hello\n"), 0o600) //nolint:errcheck
	}()

	f, err := waitOpen(ctx, p, time.Sleep)
	if err != nil {
		t.Fatalf("waitOpen: %v", err)
	}
	_ = f.Close()
}

func TestWaitOpen_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitOpen(ctx, "/nonexistent/never.log", func(time.Duration) {})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- followMerge happy path --------------------------------------------

func TestFollowMerge_OrdersAcrossStreams(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	// Pre-create empty files so tailFollow opens immediately and seeks
	// to end before any writes hit.
	if err := os.WriteFile(stdoutPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stderrPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var buf safeBuffer

	done := make(chan error, 1)
	go func() {
		done <- followMerge(ctx, &buf, []streamSource{
			{path: stdoutPath, label: "STDOUT"},
			{path: stderrPath, label: "STDERR"},
		}, filter{}, time.Sleep)
	}()

	// Give the goroutines time to seek to end.
	time.Sleep(150 * time.Millisecond)

	// Append in NON-chronological insertion order; flush window must
	// re-sort before emit.
	now := time.Now()
	appendLine(t, stderrPath, now.Add(-2*time.Second).Format(tsLayout)+" c\n")
	appendLine(t, stdoutPath, now.Add(-4*time.Second).Format(tsLayout)+" a\n")
	appendLine(t, stderrPath, now.Add(-3*time.Second).Format(tsLayout)+" b\n")

	// Wait long enough for the flush window (200ms) plus poll cadence
	// to drain everything.
	time.Sleep(800 * time.Millisecond)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("followMerge: %v", err)
	}

	out := clean(buf.String())
	idxA := strings.Index(out, " a")
	idxB := strings.Index(out, " b")
	idxC := strings.Index(out, " c")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing entries:\n%s", out)
	}
	if idxA >= idxB || idxB >= idxC {
		t.Errorf("entries out of chronological order:\n%s", out)
	}
}

// safeBuffer is a goroutine-safe bytes.Buffer for tests where the
// follow goroutines write concurrently with the assertion goroutine.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

// --- legacy split path -------------------------------------------------

func TestRunLegacySplit_BothFiles(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	writeLog(t, stdoutPath, "first stdout", "second stdout")
	writeLog(t, stderrPath, "boom err")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture os.Stdout via a pipe; legacy path writes via fmt.Printf.
	r, w, _ := os.Pipe() //nolint:errcheck
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var captured bytes.Buffer
	go func() {
		_, _ = io.Copy(&captured, r) //nolint:errcheck
		close(done)
	}()

	err := runLegacySplit(ctx, []streamSource{
		{path: stdoutPath, label: "STDOUT"},
		{path: stderrPath, label: "STDERR"},
	}, options{lines: 10})
	if err != nil {
		t.Fatalf("runLegacySplit: %v", err)
	}
	_ = w.Close()
	<-done
	out := clean(captured.String())
	if !strings.Contains(out, "second stdout") || !strings.Contains(out, "boom err") {
		t.Errorf("legacy output missing entries:\n%s", out)
	}
}

func TestTailFileLegacy_MissingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nope.log")

	r, w, _ := os.Pipe() //nolint:errcheck
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r) //nolint:errcheck
		close(done)
	}()

	tailFileLegacy(context.Background(), p, "STDOUT", 5, false, time.Sleep)
	_ = w.Close()
	<-done
	out := clean(buf.String())
	if !strings.Contains(out, "File not found") {
		t.Errorf("expected 'File not found' notice, got: %s", out)
	}
}

// --- top-level runWithContext (smoke) ---------------------------------

func TestRunWithContext_MergeSmoke(t *testing.T) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no config dir")
	}
	specDir := filepath.Join(cfgDir, "lynx", "apps")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Skip("cannot create spec dir")
	}

	tmp := t.TempDir()
	specID := "test-logs-merge-9999-9999-9999-999999999999"
	resolvedDir := filepath.Join(tmp, specID)
	if err := os.MkdirAll(resolvedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLog(t, filepath.Join(resolvedDir, "out.log"),
		"2026-04-26 12:00:01 ok-1",
		"2026-04-26 12:00:03 ok-2",
	)
	writeLog(t, filepath.Join(resolvedDir, "err.log"),
		"2026-04-26 12:00:02 boom",
	)

	specPath := filepath.Join(specDir, specID+".json")
	body := `{
		"version": 1,
		"id": "` + specID + `",
		"name": "merge-smoke-proc",
		"namespace": "default",
		"exec": {"type": "command", "command": "echo"},
		"logs": {"mode": "file", "dir": "` + tmp + `", "stdout": "out.log", "stderr": "err.log"}
	}`
	if err := os.WriteFile(specPath, []byte(body), 0o600); err != nil {
		t.Skip("cannot write spec")
	}
	defer func() { _ = os.Remove(specPath) }()

	r, w, _ := os.Pipe() //nolint:errcheck
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r) //nolint:errcheck
		close(done)
	}()

	if err := runWithContext(context.Background(), []string{"merge-smoke-proc"}); err != nil {
		t.Fatalf("runWithContext: %v", err)
	}
	_ = w.Close()
	<-done

	out := clean(buf.String())
	idx1 := strings.Index(out, "ok-1")
	idx2 := strings.Index(out, "boom")
	idx3 := strings.Index(out, "ok-2")
	if idx1 < 0 || idx2 < 0 || idx3 < 0 {
		t.Fatalf("missing entries:\n%s", out)
	}
	if idx1 >= idx2 || idx2 >= idx3 {
		t.Errorf("entries not chronologically merged:\n%s", out)
	}
}

// --- formatEntry no-ts fallback ----------------------------------------

func TestFormatEntry_NoTSPlaceholder(t *testing.T) {
	e := entry{label: "STDOUT", body: "raw", hasTS: false}
	got := clean(formatEntry(e))
	// hasTS=false → spaces of width tsLen
	if !strings.Contains(got, strings.Repeat(" ", tsLen)) {
		t.Errorf("expected placeholder spaces in %q", got)
	}
	if !strings.Contains(got, "raw") {
		t.Errorf("missing body in %q", got)
	}
}
