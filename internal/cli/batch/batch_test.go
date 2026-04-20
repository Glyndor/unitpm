package batch_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/batch"
)

func TestNewEmpty(t *testing.T) {
	r := batch.New("delete")
	if r.Op != "delete" {
		t.Errorf("Op = %q, want 'delete'", r.Op)
	}
	if r.Summary.Total != 0 || r.Summary.Ok != 0 || r.Summary.Failed != 0 {
		t.Errorf("fresh report summary non-zero: %+v", r.Summary)
	}
	if err := r.Err(); err != nil {
		t.Errorf("empty report should not report error, got %v", err)
	}
}

func TestOKAndSummary(t *testing.T) {
	r := batch.New("reset")
	r.OK("a", nil)
	r.OK("b", map[string]any{"extra": "x"})
	if r.Summary.Total != 2 || r.Summary.Ok != 2 {
		t.Errorf("summary = %+v, want total=2 ok=2", r.Summary)
	}
	if err := r.Err(); err != nil {
		t.Errorf("all-ok report should not error, got %v", err)
	}
}

func TestNoopCount(t *testing.T) {
	r := batch.New("stop")
	r.OK("running-proc", nil)
	r.Noop("already-stopped", nil)
	if r.Summary.Noop != 1 {
		t.Errorf("Summary.Noop = %d, want 1", r.Summary.Noop)
	}
	// Noop should not be counted as failed.
	if err := r.Err(); err != nil {
		t.Errorf("noop should not trigger error, got %v", err)
	}
}

func TestFailErr(t *testing.T) {
	r := batch.New("reload")
	r.Fail("ghost", errors.New("not found"))
	err := r.Err()
	if err == nil {
		t.Fatal("expected error when any target failed")
	}
	if !strings.Contains(err.Error(), "reload") {
		t.Errorf("error should mention op 'reload', got %q", err.Error())
	}
}

func TestErrMessageShapes(t *testing.T) {
	// Single target failed → "<op> failed"
	r1 := batch.New("stop")
	r1.Fail("x", errors.New("boom"))
	if got := r1.Err().Error(); got != "stop failed" {
		t.Errorf("single-target err = %q, want 'stop failed'", got)
	}

	// Mixed batch → "<op>: N of M targets failed"
	r2 := batch.New("stop")
	r2.OK("a", nil)
	r2.Fail("b", errors.New("boom"))
	r2.OK("c", nil)
	if got := r2.Err().Error(); !strings.Contains(got, "1 of 3") {
		t.Errorf("mixed err = %q, want '1 of 3'", got)
	}
}

func TestEmitJSONShape(t *testing.T) {
	r := batch.New("delete")
	r.OK("abc", map[string]any{"purged": true})
	r.Fail("ghost", errors.New("not found"))

	got := captureStdout(t, func() {
		if err := r.EmitJSON(); err != nil {
			t.Fatalf("EmitJSON returned err: %v", err)
		}
	})

	var decoded struct {
		Op      string `json:"op"`
		Results []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Error  string         `json:"error,omitempty"`
			Extra  map[string]any `json:"extra,omitempty"`
		} `json:"results"`
		Summary struct {
			Total  int `json:"total"`
			Ok     int `json:"ok"`
			Failed int `json:"failed"`
			Noop   int `json:"noop"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, got)
	}

	if decoded.Op != "delete" {
		t.Errorf("op = %q", decoded.Op)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(decoded.Results))
	}
	if decoded.Results[0].Status != "ok" || decoded.Results[1].Status != "failed" {
		t.Errorf("statuses = %q,%q", decoded.Results[0].Status, decoded.Results[1].Status)
	}
	if decoded.Results[0].Extra["purged"] != true {
		t.Errorf("expected extra.purged=true, got %v", decoded.Results[0].Extra)
	}
	if decoded.Results[1].Error != "not found" {
		t.Errorf("expected error 'not found', got %q", decoded.Results[1].Error)
	}
	if decoded.Summary.Total != 2 || decoded.Summary.Ok != 1 || decoded.Summary.Failed != 1 {
		t.Errorf("summary counts wrong: %+v", decoded.Summary)
	}
}

func TestPrintSummaryHiddenForSingle(t *testing.T) {
	r := batch.New("stop")
	r.OK("only-one", nil)
	got := captureStdout(t, r.PrintSummary)
	if got != "" {
		t.Errorf("single-target invocation should have no trailing summary, got %q", got)
	}
}

func TestPrintSummaryVisibleForBatch(t *testing.T) {
	r := batch.New("stop")
	r.OK("a", nil)
	r.OK("b", nil)
	r.Fail("c", errors.New("oops"))
	got := captureStdout(t, r.PrintSummary)
	if !strings.Contains(stripAnsi(got), "stop") {
		t.Errorf("summary should mention op, got %q", got)
	}
	if !strings.Contains(stripAnsi(got), "2 ok") || !strings.Contains(stripAnsi(got), "1 failed") {
		t.Errorf("summary should count outcomes, got %q", got)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written. Tests use this because batch writes directly to
// os.Stdout via term.Printf / fmt.Fprintln.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

func TestSplitArgs_Boolean(t *testing.T) {
	flags, pos := batch.SplitArgs([]string{"a", "--json", "b", "--purge"})
	if strings.Join(flags, ",") != "--json,--purge" {
		t.Errorf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "a,b" {
		t.Errorf("positional = %v", pos)
	}
}

func TestSplitArgsWithValues_TwoTokenForm(t *testing.T) {
	flags, pos := batch.SplitArgsWithValues(
		[]string{"a", "--namespace", "prod", "b"},
		map[string]bool{"namespace": true},
	)
	if strings.Join(flags, ",") != "--namespace,prod" {
		t.Errorf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "a,b" {
		t.Errorf("positional = %v, want a,b — value 'prod' must not leak as positional", pos)
	}
}

func TestSplitArgsWithValues_EqualsForm(t *testing.T) {
	flags, pos := batch.SplitArgsWithValues(
		[]string{"--namespace=prod", "api"},
		map[string]bool{"namespace": true},
	)
	if strings.Join(flags, ",") != "--namespace=prod" {
		t.Errorf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "api" {
		t.Errorf("positional = %v", pos)
	}
}

func TestSplitArgsWithValues_UnknownFlagFallsBackToBoolean(t *testing.T) {
	// `--json` is not in valueFlags, must not consume `next`.
	flags, pos := batch.SplitArgsWithValues(
		[]string{"--json", "next"},
		map[string]bool{"namespace": true},
	)
	if strings.Join(flags, ",") != "--json" {
		t.Errorf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "next" {
		t.Errorf("positional = %v", pos)
	}
}

func TestSplitArgsWithValues_TrailingValueFlagWithoutValue(t *testing.T) {
	// Don't index past end — the flag package will error later anyway.
	flags, pos := batch.SplitArgsWithValues(
		[]string{"api", "--namespace"},
		map[string]bool{"namespace": true},
	)
	if strings.Join(flags, ",") != "--namespace" {
		t.Errorf("flags = %v", flags)
	}
	if strings.Join(pos, ",") != "api" {
		t.Errorf("positional = %v", pos)
	}
}

func stripAnsi(s string) string {
	var b strings.Builder
	inSeq := false
	for _, r := range s {
		if r == '\033' {
			inSeq = true
			continue
		}
		if inSeq {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inSeq = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
