package term

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	func() {
		defer func() { _ = w.Close() }()
		fn()
	}()
	<-done
	return buf.String()
}

func TestColorStringHelpers(t *testing.T) {
	for _, fn := range []func(string, ...any) string{
		RedString, GreenString, YellowString, BlueString,
		CyanString, MagentaString, BoldString, DimString,
	} {
		got := fn("hello %s", "world")
		if !strings.Contains(got, "hello world") {
			t.Errorf("color helper dropped substring: %q", got)
		}
	}
}

func TestStyler_Methods(t *testing.T) {
	s := NewStyler()
	// Methods just format — behavior depends on Enabled().
	for _, m := range []func(string, ...any) string{
		s.Red, s.Green, s.Yellow, s.Blue, s.Cyan, s.Magenta, s.Bold, s.Dim,
	} {
		if !strings.Contains(m("x %d", 1), "x 1") {
			t.Error("styler dropped format args")
		}
	}
}

func TestStyler_Enabled(t *testing.T) {
	// Enabled only returns true when stdout is a TTY; under 'go test'
	// stdout is piped so it should be false.
	s := NewStyler()
	_ = s.Enabled() // just exercise the branch
}

func TestStyler_Colorize_Disabled(t *testing.T) {
	s := &Styler{enabled: false}
	got := s.Colorize("\033[31m", "x")
	if got != "x" {
		t.Errorf("disabled styler should not emit escape codes, got %q", got)
	}
}

func TestStyler_Colorize_Enabled(t *testing.T) {
	s := &Styler{enabled: true}
	got := s.Colorize("\033[31m", "x")
	if !strings.Contains(got, "\033[31m") || !strings.Contains(got, "x") || !strings.Contains(got, "\033[0m") {
		t.Errorf("enabled styler should wrap with escapes, got %q", got)
	}
}

func TestPrintf_Println(t *testing.T) {
	SetQuiet(false)
	out := captureStdout(t, func() {
		_, _ = Printf("hello %s\n", "x")
		_, _ = Println("bye")
	})
	if !strings.Contains(out, "hello x") || !strings.Contains(out, "bye") {
		t.Errorf("output missing: %q", out)
	}
}

func TestSetQuiet_Suppresses(t *testing.T) {
	SetQuiet(true)
	t.Cleanup(func() { SetQuiet(false) })
	if !IsQuiet() {
		t.Error("IsQuiet should be true")
	}
	out := captureStdout(t, func() {
		_, _ = Printf("should-not-appear\n")
		_, _ = Println("also-suppressed")
	})
	if out != "" {
		t.Errorf("quiet mode should swallow output, got %q", out)
	}
}

func TestIsTTY_ShouldUseColor(t *testing.T) {
	// Under 'go test' stdout is a pipe → not a TTY → no color.
	_ = IsTTY()
	_ = ShouldUseColor()
}
