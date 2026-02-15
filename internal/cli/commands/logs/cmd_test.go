package logs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/jsonx"
)

func TestRun(t *testing.T) {
	// Setup temp environment
	tmpDir := t.TempDir()
	configHome := filepath.Join(tmpDir, ".config")
	stateHome := filepath.Join(tmpDir, ".local/state")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	// Create directories
	specDir := filepath.Join(configHome, "lynx", "apps")
	logDir := filepath.Join(stateHome, "lynx", "logs", "testapp")
	if err := os.MkdirAll(specDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create spec
	spec := protocol.AppSpec{
		ID:   "testapp",
		Name: "Test App",
		Logs: &protocol.AppLogs{
			Dir:    "", // default
			Stdout: "stdout.log",
			Stderr: "stderr.log",
		},
	}
	specData, _ := jsonx.Marshal(spec)
	if err := os.WriteFile(filepath.Join(specDir, "testapp.json"), specData, 0600); err != nil {
		t.Fatal(err)
	}

	// Create log files with known content
	// Write 10 lines to stdout
	stdoutContent := ""
	for i := 1; i <= 10; i++ {
		stdoutContent += fmt.Sprintf("OUT Line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(logDir, "stdout.log"), []byte(stdoutContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Write 10 lines to stderr
	stderrContent := ""
	for i := 1; i <= 10; i++ {
		stderrContent += fmt.Sprintf("ERR Line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(logDir, "stderr.log"), []byte(stderrContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Helper to capture stdout
	captureStdout := func(f func() error) (string, error) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := f()

		w.Close()
		os.Stdout = oldStdout
		out, _ := io.ReadAll(r)
		return string(out), err
	}

	// Test 1: Default (both streams, last 200 lines)
	t.Run("Default", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return Run([]string{"testapp"})
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if !strings.Contains(out, "OUT Line 1") || !strings.Contains(out, "OUT Line 10") {
			t.Errorf("Missing stdout lines in output: %s", out)
		}
		if !strings.Contains(out, "ERR Line 1") || !strings.Contains(out, "ERR Line 10") {
			t.Errorf("Missing stderr lines in output: %s", out)
		}
	})

	// Test 2: Lines limit
	t.Run("Lines Limit", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return Run([]string{"testapp", "--lines", "2"})
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		// Should have Line 9 and 10, but not Line 8
		if !strings.Contains(out, "OUT Line 9") || !strings.Contains(out, "OUT Line 10") {
			t.Errorf("Missing last lines: %s", out)
		}
		if strings.Contains(out, "OUT Line 8") {
			t.Errorf("Too many lines: %s", out)
		}
	})

	// Test 3: Stdout only
	t.Run("Stdout Only", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return Run([]string{"testapp", "--stdout"})
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if !strings.Contains(out, "OUT Line 1") {
			t.Errorf("Missing stdout: %s", out)
		}
		if strings.Contains(out, "ERR Line") {
			t.Errorf("Should not contain stderr: %s", out)
		}
	})

	// Test 4: Stderr only
	t.Run("Stderr Only", func(t *testing.T) {
		out, err := captureStdout(func() error {
			return Run([]string{"testapp", "--stderr"})
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if !strings.Contains(out, "ERR Line 1") {
			t.Errorf("Missing stderr: %s", out)
		}
		if strings.Contains(out, "OUT Line") {
			t.Errorf("Should not contain stdout: %s", out)
		}
	})
}
