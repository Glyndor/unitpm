package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLogDir(t *testing.T) {
	// Mock HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// Clear XDG_STATE_HOME to test fallback
	t.Setenv("XDG_STATE_HOME", "")
	if os.PathSeparator == '\\' {
		t.Setenv("USERPROFILE", tmpHome)
	}

	// Mock EUID
	originalEuid := currentEuid
	defer func() { currentEuid = originalEuid }()

	// Case 1: Non-root user, default dir
	currentEuid = func() int { return 1000 }
	dir, err := GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}

	expected := filepath.Join(tmpHome, ".local/state/lynx/logs")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}

	// Case 2: Root user, default dir (Skip on Windows as LogRoot might be invalid)
	if os.PathSeparator == '/' {
		currentEuid = func() int { return 0 }
		dir, err = GetLogDir("")
		if err != nil {
			t.Fatalf("GetLogDir (root) failed: %v", err)
		}
		if dir != LogRoot {
			t.Errorf("Expected %s, got %s", LogRoot, dir)
		}
	}
}

func TestResolveConfiguredDir(t *testing.T) {
	originalEuid := currentEuid
	defer func() { currentEuid = originalEuid }()

	// Case 1: Non-root user can use any path
	currentEuid = func() int { return 1000 }
	customDir := filepath.Join(os.TempDir(), "lynx-logs")
	dir, err := GetLogDir(customDir)
	if err != nil {
		t.Fatalf("GetLogDir (user custom) failed: %v", err)
	}
	if dir != filepath.Clean(customDir) {
		t.Errorf("Expected %s, got %s", filepath.Clean(customDir), dir)
	}
}

func TestResolveLogPaths(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if os.PathSeparator == '\\' {
		t.Setenv("USERPROFILE", tmpHome)
	}
	t.Setenv("XDG_STATE_HOME", "")

	originalEuid := currentEuid
	defer func() { currentEuid = originalEuid }()
	currentEuid = func() int { return 1000 }

	specID := "test-app"
	stdoutSpec := "out.log"
	stderrSpec := "err.log"

	stdout, stderr, err := ResolveLogPaths(specID, "", stdoutSpec, stderrSpec)
	if err != nil {
		t.Fatalf("ResolveLogPaths failed: %v", err)
	}

	baseDir := filepath.Join(tmpHome, ".local/state/lynx/logs", "test-app")
	expectedStdout := filepath.Join(baseDir, "out.log")
	expectedStderr := filepath.Join(baseDir, "err.log")

	if stdout != expectedStdout {
		t.Errorf("Expected stdout %s, got %s", expectedStdout, stdout)
	}
	if stderr != expectedStderr {
		t.Errorf("Expected stderr %s, got %s", expectedStderr, stderr)
	}
}

func TestPathTraversal(t *testing.T) {
	originalEuid := currentEuid
	defer func() { currentEuid = originalEuid }()
	currentEuid = func() int { return 1000 }

	// Attempt path traversal
	_, err := GetLogDir("../../../etc/passwd")
	if err == nil {
		t.Error("GetLogDir should fail on path traversal")
	}
	if err.Error() != "invalid log dir" {
		t.Errorf("Unexpected error: %v", err)
	}
}
