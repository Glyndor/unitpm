package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestGetLogDir(t *testing.T) {
	// Mock HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// Clear XDG_STATE_HOME to test fallback
	t.Setenv("XDG_STATE_HOME", "")

	// Mock EUID
	// Note: We can't easily mock os.Geteuid in a cross-platform way without refactoring the code to use a variable for the function.
	// The provided code already does: var getEuid = os.Geteuid
	// So we can swap it out.

	originalGetEuid := getEuid
	defer func() { getEuid = originalGetEuid }()

	// Case 1: Non-root user, default dir
	getEuid = func() int { return 1000 }
	dir, err := GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}
	// The path might contain temporary directory components on Windows/Mac, so we check suffixes or full path logic
	// On Windows, the tmpDir is typically in AppData/Local/Temp, which is NOT user home.
	// But GetLogDir uses os.UserHomeDir() when XDG_STATE_HOME is not set.
	// So we need to check against os.UserHomeDir().
	
	realHome, _ := os.UserHomeDir()
	expected := filepath.Join(realHome, ".local/state/lynx/logs")
	// If HOME is overridden, os.UserHomeDir on unix respects it. On Windows it might not if USERPROFILE is not set.
	// Let's rely on what GetLogDir returned and verifying it matches what we expect from the logic.
	// The logic is: $HOME/.local/state/lynx/logs
	
	// If we are on windows, t.Setenv("HOME", ...) might not be enough for os.UserHomeDir()
	if os.PathSeparator == '\\' {
		t.Setenv("USERPROFILE", tmpHome)
	}
	
	// Re-run GetLogDir after setting env vars
	dir, err = GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}
	
	expected = filepath.Join(tmpHome, ".local/state/lynx/logs")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}

	// Case 2: Root user, default dir
	getEuid = func() int { return 0 }
	dir, err = GetLogDir("")
	if err != nil {
		t.Fatalf("GetLogDir (root) failed: %v", err)
	}
	// LogRoot is /var/log/lynx-pm
	if dir != LogRoot {
		t.Errorf("Expected %s, got %s", LogRoot, dir)
	}
}

func TestResolveConfiguredDir(t *testing.T) {
	originalGetEuid := getEuid
	defer func() { getEuid = originalGetEuid }()

	// Case 1: Non-root user can use any path
	getEuid = func() int { return 1000 }
	customDir := filepath.Join(os.TempDir(), "lynx-logs")
	dir, err := GetLogDir(customDir)
	if err != nil {
		t.Fatalf("GetLogDir (user custom) failed: %v", err)
	}
	if dir != filepath.Clean(customDir) {
		t.Errorf("Expected %s, got %s", filepath.Clean(customDir), dir)
	}

	// Case 2: Root user restricted to allowed roots
	getEuid = func() int { return 0 }
	
	// Valid root path
	// On Windows, LogRoot (/var/log/lynx-pm) is considered relative or invalid, 
	// so filepath.Join behaves differently. We skip this part on Windows if LogRoot isn't absolute.
	if filepath.IsAbs(LogRoot) {
		validDir := filepath.Join(LogRoot, "subdir")
		dir, err = GetLogDir(validDir)
		if err != nil {
			t.Errorf("GetLogDir (root valid) failed: %v", err)
		} else if dir != filepath.Clean(validDir) {
			t.Errorf("Expected %s, got %s", filepath.Clean(validDir), dir)
		}
	} else {
		t.Skip("Skipping root path test on non-Linux environment where LogRoot is not absolute")
	}

	// Invalid root path (outside allowed roots)
	invalidDir := "/tmp/hacker"
	// Only test if we are on a system where /tmp is absolute
	if filepath.IsAbs(invalidDir) {
		_, err = GetLogDir(invalidDir)
		if err == nil {
			t.Error("GetLogDir (root invalid) should fail")
		}
	}
}

func TestResolveLogPaths(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if os.PathSeparator == '\\' {
		t.Setenv("USERPROFILE", tmpHome)
	}
	t.Setenv("XDG_STATE_HOME", "")
	
	originalGetEuid := getEuid
	defer func() { getEuid = originalGetEuid }()
	getEuid = func() int { return 1000 }

	spec := &protocol.AppSpec{
		ID: "test-app",
		Logs: &protocol.AppLogs{
			Stdout: "out.log",
			Stderr: "err.log",
		},
	}

	stdout, stderr, err := ResolveLogPaths(spec)
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
	originalGetEuid := getEuid
	defer func() { getEuid = originalGetEuid }()
	getEuid = func() int { return 1000 }

	// Attempt path traversal
	_, err := GetLogDir("../../../etc/passwd")
	if err == nil {
		t.Error("GetLogDir should fail on path traversal")
	}
	if err.Error() != "invalid log dir" {
		t.Errorf("Unexpected error: %v", err)
	}
}
