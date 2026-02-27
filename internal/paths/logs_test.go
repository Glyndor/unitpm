//go:build linux

package paths

import (
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
		// Root should be restricted to /var/log/lynx-pm
		err := ValidateLogDir("/tmp")
		if err == nil {
			t.Error("ValidateLogDir(/tmp) = nil; want error")
		}
	})
}

func ValidateLogDir(dir string) error {
	_, err := GetLogDir(dir)
	return err
}

func TestGetLogDirRootAcceptsAllowedRoots(t *testing.T) {
	withEuid(t, 0, func() {
		// Test system root
		if err := ValidateLogDir(LogRoot); err != nil {
			t.Errorf("ValidateLogDir(%s) = %v; want nil", LogRoot, err)
		}

		// Test subdirectory
		sub := filepath.Join(LogRoot, "subdir")
		if err := ValidateLogDir(sub); err != nil {
			t.Errorf("ValidateLogDir(%s) = %v; want nil", sub, err)
		}
	})
}
