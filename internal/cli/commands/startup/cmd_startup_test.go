//go:build linux

package startup //nolint:testpackage

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLinuxStartup(t *testing.T) {
	// Restore variables after tests
	originalGetEuid := getEuid
	originalStat := stat
	originalLookPath := lookPath

	defer func() {
		getEuid = originalGetEuid
		stat = originalStat
		lookPath = originalLookPath
	}()

	// Mock helpers
	mockStatExists := func(_ string) (os.FileInfo, error) {
		return nil, nil // Exists
	}
	mockStatNotExists := func(_ string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	mockLookPathFound := func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	mockLookPathNotFound := func(_ string) (string, error) {
		return "", errors.New("executable file not found")
	}

	t.Run("Non-root returns error", func(t *testing.T) {
		getEuid = func() int { return 1000 }
		// Mocks for systemd (shouldn't reach here but good to be safe)
		stat = mockStatExists
		lookPath = mockLookPathFound

		runner := &MockRunner{}
		err := Run(runner, []string{})
		if err == nil {
			t.Error("Expected error for non-root, got nil")
		}
		if err.Error() != "admin privileges required" {
			t.Errorf("Expected 'admin privileges required', got %v", err)
		}
	})

	t.Run("Unsupported OS (Systemd missing)", func(t *testing.T) {
		getEuid = func() int { return 0 }
		stat = mockStatNotExists
		lookPath = mockLookPathFound

		runner := &MockRunner{}
		err := Run(runner, []string{})
		if err == nil {
			t.Error("Expected error for unsupported OS, got nil")
		}
		if !strings.Contains(err.Error(), "ERR_UNSUPPORTED") {
			t.Errorf("Expected ERR_UNSUPPORTED, got %v", err)
		}
	})

	t.Run("Unsupported OS (systemctl missing)", func(t *testing.T) {
		getEuid = func() int { return 0 }
		stat = mockStatExists
		lookPath = mockLookPathNotFound

		runner := &MockRunner{}
		err := Run(runner, []string{})
		if err == nil {
			t.Error("Expected error for unsupported OS, got nil")
		}
		if !strings.Contains(err.Error(), "ERR_UNSUPPORTED") {
			t.Errorf("Expected ERR_UNSUPPORTED, got %v", err)
		}
	})

	t.Run("Root success", func(t *testing.T) {
		getEuid = func() int { return 0 }
		stat = mockStatExists
		lookPath = mockLookPathFound

		runner := &MockRunner{
			Responses: map[string]MockResult{
				"systemctl is-active": {Stdout: "active\n"},
			},
		}
		err := Run(runner, []string{})
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// Verify calls
		expectedCalls := []string{
			"systemctl daemon-reload",
			"systemctl enable --now lynx.lynxd.service",
			"systemctl is-active lynx.lynxd.service",
		}
		if len(runner.Calls) != len(expectedCalls) {
			t.Errorf("Expected %d calls, got %d", len(expectedCalls), len(runner.Calls))
		}
		for i, call := range runner.Calls {
			if call != expectedCalls[i] {
				t.Errorf("Call %d: expected %q, got %q", i, expectedCalls[i], call)
			}
		}
	})

	t.Run("Service inactive", func(t *testing.T) {
		getEuid = func() int { return 0 }
		stat = mockStatExists
		lookPath = mockLookPathFound

		runner := &MockRunner{
			Responses: map[string]MockResult{
				"systemctl is-active": {Stdout: "inactive\n", ExitCode: 3},
			},
		}
		err := Run(runner, []string{})
		if err == nil {
			t.Error("Expected error for inactive service, got nil")
		}
		if !strings.Contains(err.Error(), "lynxd service is not active") {
			t.Errorf("Expected inactive service error, got %v", err)
		}
	})

	t.Run("Systemctl failure", func(t *testing.T) {
		getEuid = func() int { return 0 }
		stat = mockStatExists
		lookPath = mockLookPathFound

		runner := &MockRunner{
			Responses: map[string]MockResult{
				"systemctl daemon-reload": {Err: errors.New("failed"), Stderr: "access denied"},
			},
		}
		err := Run(runner, []string{})
		if err == nil {
			t.Error("Expected error for systemctl failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to reload daemon") {
			t.Errorf("Expected reload failure error, got %v", err)
		}
	})
}
