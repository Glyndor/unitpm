package completion_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/completion"
)

// captureStdout runs fn with os.Stdout rerouted to an in-memory buffer and
// returns what was written. Panic-safe: os.Stdout is always restored and the
// reader goroutine always terminates, even if fn panics.
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
		defer func() { _ = w.Close() }() // unblock the copier even on panic
		fn()
	}()
	<-done
	return buf.String()
}

func TestRun_Help(t *testing.T) {
	if err := completion.Run([]string{"--help"}); err != nil {
		t.Errorf("--help returned error: %v", err)
	}
}

func TestRun_MissingShell(t *testing.T) {
	err := completion.Run([]string{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_UnsupportedShell(t *testing.T) {
	err := completion.Run([]string{"tcsh"})
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Bash(t *testing.T) {
	out := captureStdout(t, func() {
		if err := completion.Run([]string{"bash"}); err != nil {
			t.Errorf("bash: %v", err)
		}
	})
	for _, want := range []string{"_lynx_completions", "complete -F", "start", "stop"} {
		if !strings.Contains(out, want) {
			t.Errorf("bash script missing %q", want)
		}
	}
	// Hidden internals must not leak
	for _, bad := range []string{"_exec-env", "_exec-sandbox"} {
		if strings.Contains(out, bad) {
			t.Errorf("bash script leaked hidden command %q", bad)
		}
	}
}

func TestRun_Zsh(t *testing.T) {
	out := captureStdout(t, func() {
		if err := completion.Run([]string{"zsh"}); err != nil {
			t.Errorf("zsh: %v", err)
		}
	})
	if !strings.Contains(out, "#compdef lynx") {
		t.Error("zsh script missing #compdef directive")
	}
	if !strings.Contains(out, "start") {
		t.Error("zsh script missing 'start'")
	}
}

func TestRun_Fish(t *testing.T) {
	// Fish script dynamic portion depends on registry.GetAll(); the
	// static tail does not. Assert on markers that exist either way.
	out := captureStdout(t, func() {
		if err := completion.Run([]string{"fish"}); err != nil {
			t.Errorf("fish: %v", err)
		}
	})
	for _, want := range []string{"__lynx_list_names", "lynx list --long", "completion"} {
		if !strings.Contains(out, want) {
			t.Errorf("fish script missing %q", want)
		}
	}
}

func TestGetSpec(t *testing.T) {
	spec := completion.GetSpec()
	if spec.Name != "completion" {
		t.Errorf("expected name 'completion', got %s", spec.Name)
	}
}

func TestPrintHelp(t *testing.T) {
	// ensure no panic
	_ = captureStdout(t, func() { completion.PrintHelp() })
}
