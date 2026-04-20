package table_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/format"
	"github.com/Jaro-c/Lynx/internal/cli/table"
)

func TestTableRenderBasic(t *testing.T) {
	got := captureStdout(t, func() {
		tbl := table.New([]string{"id", "name", "state"})
		tbl.AddRow([]string{"abc12345", "api", "running"})
		tbl.AddRow([]string{"def67890", "worker", "stopped"})
		tbl.Render()
	})
	plain := format.StripAnsi(got)
	// Structure checks.
	for _, want := range []string{"id", "name", "state", "abc12345", "api", "running", "worker", "stopped"} {
		if !strings.Contains(plain, want) {
			t.Errorf("table output missing %q; got:\n%s", want, plain)
		}
	}
	// Should use box-drawing chars on both ends.
	if !strings.Contains(plain, "┌") || !strings.Contains(plain, "└") {
		t.Errorf("expected box borders, got:\n%s", plain)
	}
}

func TestKVSkipsEmptyRows(t *testing.T) {
	// Empty values should not render rows. Rendering an all-empty KV emits nothing.
	got := captureStdout(t, func() {
		table.KV("Hidden", []table.KVRow{
			{"a", ""},
			{"b", ""},
		})
	})
	if got != "" {
		t.Errorf("all-empty KV should render nothing, got:\n%s", got)
	}
}

func TestKVTitleAndContent(t *testing.T) {
	got := captureStdout(t, func() {
		table.KV("Process", []table.KVRow{
			{"state", "running"},
			{"pid", "1234"},
			{"omitted", ""}, // should be dropped
		})
	})
	plain := format.StripAnsi(got)
	for _, want := range []string{"Process", "state", "running", "pid", "1234"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output missing %q; got:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "omitted") {
		t.Errorf("empty row should have been dropped; got:\n%s", plain)
	}
}

func TestTableSetMaxColWidthsIgnoresMismatch(t *testing.T) {
	// Wrong-length slice should not panic; widths are silently ignored.
	tbl := table.New([]string{"a", "b"})
	tbl.SetMaxColWidths([]int{5, 5, 5}) // 3 widths for 2 headers
	tbl.AddRow([]string{"1", "2"})
	_ = captureStdout(t, tbl.Render)
}

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
