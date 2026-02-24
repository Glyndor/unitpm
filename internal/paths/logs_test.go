//go:build linux

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withEuid(t *testing.T, euid int, fn func()) {
	t.Helper()

	origGetEuid := getEuid
	getEuid = func() int {
		return euid
	}
	defer func() {
		getEuid = origGetEuid
	}()

	fn()
}

func TestGetLogDirRootRejectsRelative(t *testing.T) {
	withEuid(t, 0, func() {
		if _, err := GetLogDir("relative/path"); err == nil {
			t.Fatalf("expected error for relative path in root mode")
		} else if !strings.Contains(err.Error(), "must be absolute when running as root") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestGetLogDirRootRejectsOutsideAllowedRoots(t *testing.T) {
	withEuid(t, 0, func() {
		if _, err := GetLogDir("/opt/lynxlogs"); err == nil {
			t.Fatalf("expected error for path outside allowed roots")
		} else if !strings.Contains(err.Error(), "invalid log dir: outside allowed roots") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestGetLogDirRootAcceptsAllowedRoots(t *testing.T) {
	withEuid(t, 0, func() {
		dir, err := GetLogDir("/var/log/lynx")
		if err != nil {
			t.Fatalf("expected /var/log/lynx to be accepted, got error: %v", err)
		}
		if dir != "/var/log/lynx" {
			t.Fatalf("expected /var/log/lynx, got %s", dir)
		}

		stateHome := t.TempDir()
		if err := os.Setenv("XDG_STATE_HOME", stateHome); err != nil {
			t.Fatalf("failed to set XDG_STATE_HOME: %v", err)
		}
		defer os.Unsetenv("XDG_STATE_HOME")

		custom := filepath.Join(stateHome, "lynx", "logs")
		dir, err = GetLogDir(custom)
		if err != nil {
			t.Fatalf("expected %s to be accepted, got error: %v", custom, err)
		}
		if dir != custom {
			t.Fatalf("expected %s, got %s", custom, dir)
		}
	})
}
