package logs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeBanner builds the same 3-line block writeBanner would emit for
// (event, ts). Width is fixed at 80 to match daemon/manager/logwriter.go.
func makeBanner(event, tsStr string) string {
	const width = 80
	left := "==  " + event + "  "
	right := "  " + tsStr + "  =="
	fillN := width - len(left) - len(right)
	if fillN < 4 {
		fillN = 4
	}
	rule := strings.Repeat("=", width)
	mid := left + strings.Repeat("=", fillN) + right
	return rule + "\n" + mid + "\n" + rule
}

func TestIsBannerRule(t *testing.T) {
	cases := map[string]bool{
		strings.Repeat("=", 80):       true,
		strings.Repeat("=", 8):        true,
		"=======":                     false, // 7 chars: too short
		"":                            false,
		"== STARTED ==":               false,
		"=" + strings.Repeat(" ", 80): false,
	}
	for in, want := range cases {
		if got := isBannerRule(in); got != want {
			t.Errorf("isBannerRule(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseBannerMiddle(t *testing.T) {
	mid := "==  STARTED                                              2026-04-26 12:00:00  =="
	ts, ok := parseBannerMiddle(mid)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if ts.Year() != 2026 || ts.Hour() != 12 {
		t.Errorf("ts wrong: %v", ts)
	}

	if _, ok := parseBannerMiddle("==  STARTED  =="); ok {
		t.Error("missing ts must not parse")
	}
	if _, ok := parseBannerMiddle(""); ok {
		t.Error("empty must not parse")
	}
}

func TestReadEntries_BannerSurfacesAsEntry(t *testing.T) {
	body := "2026-04-26 11:59:59 before\n" +
		makeBanner("STARTED", "2026-04-26 12:00:00") + "\n" +
		"2026-04-26 12:00:01 after\n"
	entries, _ := readEntries(strings.NewReader(body), "STDOUT", 0)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3:\n%+v", len(entries), entries)
	}
	if !strings.Contains(entries[1].body, "STARTED") {
		t.Errorf("banner entry body missing STARTED:\n%q", entries[1].body)
	}
	if entries[1].ts.Hour() != 12 || entries[1].ts.Minute() != 0 {
		t.Errorf("banner ts wrong: %v", entries[1].ts)
	}
	if !entries[0].ts.Before(entries[1].ts) || !entries[1].ts.Before(entries[2].ts) {
		t.Errorf("banner not chronologically ordered: %v / %v / %v",
			entries[0].ts, entries[1].ts, entries[2].ts)
	}
}

func TestReadEntries_MultipleLifecycleBanners(t *testing.T) {
	body := makeBanner("STARTED", "2026-04-26 12:00:00") + "\n" +
		"2026-04-26 12:00:30 working\n" +
		makeBanner("RESTARTED", "2026-04-26 12:01:00") + "\n" +
		"2026-04-26 12:01:30 working again\n" +
		makeBanner("STOPPED", "2026-04-26 12:02:00") + "\n"
	entries, _ := readEntries(strings.NewReader(body), "STDOUT", 0)
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}
	wantContains := []string{"STARTED", "working", "RESTARTED", "working again", "STOPPED"}
	for i, w := range wantContains {
		if !strings.Contains(entries[i].body, w) {
			t.Errorf("[%d] body = %q, want contains %q", i, entries[i].body, w)
		}
	}
}

func TestStreamMerge_BannersInterleaved(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")

	stdoutBody := "2026-04-26 12:00:00 ok-1\n" +
		makeBanner("RESTARTED", "2026-04-26 12:00:02") + "\n" +
		"2026-04-26 12:00:03 ok-2\n"
	stderrBody := "2026-04-26 12:00:01 err-1\n"

	if err := os.WriteFile(stdoutPath, []byte(stdoutBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stderrPath, []byte(stderrBody), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := streamMerge(context.Background(), &buf, filter{},
		streamSource{path: stdoutPath, label: "STDOUT"},
		streamSource{path: stderrPath, label: "STDERR"},
	)
	if err != nil {
		t.Fatalf("streamMerge: %v", err)
	}
	out := clean(buf.String())
	idxOK1 := strings.Index(out, "ok-1")
	idxErr1 := strings.Index(out, "err-1")
	idxBanner := strings.Index(out, "RESTARTED")
	idxOK2 := strings.Index(out, "ok-2")
	if idxOK1 < 0 || idxErr1 < 0 || idxBanner < 0 || idxOK2 < 0 {
		t.Fatalf("missing entry:\n%s", out)
	}
	if idxOK1 >= idxErr1 || idxErr1 >= idxBanner || idxBanner >= idxOK2 {
		t.Errorf("banner not chronologically merged across streams:\n%s", out)
	}
}

func TestBoundedTail_BannerCountsTowardsLimit(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")

	var b bytes.Buffer
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "2026-04-26 12:00:%02d entry-%d\n", i, i)
	}
	b.WriteString(makeBanner("STOPPED", "2026-04-26 12:00:30"))
	b.WriteString("\n")
	if err := os.WriteFile(stdoutPath, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := boundedTail(&out, []streamSource{{path: stdoutPath, label: "STDOUT"}}, 5, filter{}); err != nil {
		t.Fatal(err)
	}
	got := clean(out.String())
	if !strings.Contains(got, "STOPPED") {
		t.Errorf("banner missing from bounded tail:\n%s", got)
	}
}

// TestBannerSplitOK checks the iterator handles a banner appearing at
// the very end of the lookahead window (across refill boundaries).
func TestStreamMerge_BannerAtEOF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trailing.log")
	body := "2026-04-26 12:00:00 hello\n" +
		makeBanner("EXITED  code=0", "2026-04-26 12:00:01") + "\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := streamMerge(context.Background(), &buf, filter{},
		streamSource{path: p, label: "STDOUT"}); err != nil {
		t.Fatal(err)
	}
	out := clean(buf.String())
	if !strings.Contains(out, "hello") {
		t.Errorf("missing pre-banner entry:\n%s", out)
	}
	if !strings.Contains(out, "EXITED") {
		t.Errorf("missing banner:\n%s", out)
	}
}
