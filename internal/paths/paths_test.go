//go:build linux

package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jaro-c/Lynx/internal/paths"
)

// TestResolveLogPaths_DefaultPaths verifies that default log paths are resolved correctly
// for non-root users (euid != 0).
func TestResolveLogPaths_DefaultPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	specID := "test-proc-id"
	stdout, stderr, err := paths.ResolveLogPaths(specID, "", "", "")
	if err != nil {
		t.Fatalf("ResolveLogPaths error: %v", err)
	}

	expectedDir := filepath.Join(tmp, "lynx/logs", specID)
	if filepath.Dir(stdout) != expectedDir {
		t.Errorf("stdout dir = %q, want %q", filepath.Dir(stdout), expectedDir)
	}
	if filepath.Base(stdout) != "stdout.log" {
		t.Errorf("stdout file = %q, want stdout.log", filepath.Base(stdout))
	}
	if filepath.Base(stderr) != "stderr.log" {
		t.Errorf("stderr file = %q, want stderr.log", filepath.Base(stderr))
	}
}

func TestResolveLogPaths_CustomDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	customDir := filepath.Join(tmp, "myapp/logs")
	specID := "proc-abc"

	stdout, stderr, err := paths.ResolveLogPaths(specID, customDir, "", "")
	if err != nil {
		t.Fatalf("ResolveLogPaths error: %v", err)
	}

	expectedBase := filepath.Join(customDir, specID)
	if filepath.Dir(stdout) != expectedBase {
		t.Errorf("stdout dir = %q, want %q", filepath.Dir(stdout), expectedBase)
	}
	if filepath.Dir(stderr) != expectedBase {
		t.Errorf("stderr dir = %q, want %q", filepath.Dir(stderr), expectedBase)
	}
}

func TestResolveLogPaths_CustomFilenames(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	stdout, stderr, err := paths.ResolveLogPaths("proc-1", "", "out.txt", "err.txt")
	if err != nil {
		t.Fatalf("ResolveLogPaths error: %v", err)
	}

	if filepath.Base(stdout) != "out.txt" {
		t.Errorf("stdout = %q, want base out.txt", stdout)
	}
	if filepath.Base(stderr) != "err.txt" {
		t.Errorf("stderr = %q, want base err.txt", stderr)
	}
}

func TestResolveLogPaths_AbsoluteCustomFilename(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	absStdout := filepath.Join(tmp, "custom", "out.log")
	stdout, _, err := paths.ResolveLogPaths("proc-1", "", absStdout, "")
	if err != nil {
		t.Fatalf("ResolveLogPaths error: %v", err)
	}
	if stdout != absStdout {
		t.Errorf("stdout = %q, want %q (absolute)", stdout, absStdout)
	}
}

func TestResolveLogPaths_PathTooLong(t *testing.T) {
	longDir := string(make([]byte, 5000))
	_, _, err := paths.ResolveLogPaths("proc-1", longDir, "", "")
	if err == nil {
		t.Error("expected error for path too long, got nil")
	}
}

func TestResolveLogPaths_DotDotDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	_, _, err := paths.ResolveLogPaths("proc-1", "../escape", "", "")
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

func TestGetLogDir_XDGStateHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir, err := paths.GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir error: %v", err)
	}

	expected := filepath.Join(tmp, "lynx/logs")
	if dir != expected {
		t.Errorf("log dir = %q, want %q", dir, expected)
	}
}

func TestGetLogDir_FallbackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	_ = os.Unsetenv("XDG_STATE_HOME")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}

	dir, err := paths.GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir error: %v", err)
	}

	expected := filepath.Join(home, ".local/state/lynx/logs")
	if dir != expected {
		t.Errorf("log dir = %q, want %q", dir, expected)
	}
}

func TestGetLogDir_CustomDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	customDir := filepath.Join(tmp, "mydir")
	dir, err := paths.GetLogDir(customDir)
	if err != nil {
		t.Fatalf("GetLogDir error: %v", err)
	}
	if dir != customDir {
		t.Errorf("dir = %q, want %q", dir, customDir)
	}
}
