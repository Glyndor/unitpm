package logs_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/logs"
)

func TestRun_MissingTarget(t *testing.T) {
	err := logs.Run([]string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_FlagsOnly(t *testing.T) {
	// All flags, no target → still missing target
	err := logs.Run([]string{"--follow", "--lines", "50"})
	if err == nil {
		t.Fatal("expected error for flags-only args")
	}
	if !strings.Contains(err.Error(), "missing process ID or name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_ProcessNotFound(t *testing.T) {
	// Spec dir is empty in test env; any name should return "not found"
	err := logs.Run([]string{"nonexistent-process-xyz"})
	if err == nil {
		t.Fatal("expected error for unknown process")
	}
	// Either "not found" or "failed to load specs"
	msg := err.Error()
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "failed to load") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := logs.GetSpec()
	if spec.Name != "logs" {
		t.Errorf("expected name 'logs', got %s", spec.Name)
	}
	if len(spec.Aliases) == 0 {
		t.Error("expected at least one alias")
	}
}

func TestRun_NamespacedTarget_NotFound(t *testing.T) {
	// namespace:name syntax with no matching spec
	err := logs.Run([]string{"ns:app"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "failed to load") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestTailFile_FileNotFound verifies logs don't panic when log file is absent.
// Runs without --follow so it returns immediately.
func TestRun_ExistingSpec_MissingLogFile(t *testing.T) {
	// Write a minimal spec file so logs can find the process
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine config dir")
	}
	specDir := filepath.Join(cfgDir, "lynx", "apps")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Skip("cannot create spec dir")
	}

	// Write a minimal spec with a known ID
	specID := "test-logs-0000-0000-0000-000000000001"
	specPath := filepath.Join(specDir, specID+".json")
	specContent := `{
		"version": 1,
		"id": "` + specID + `",
		"name": "test-logs-proc",
		"namespace": "default",
		"exec": {"type": "command", "command": "echo"},
		"logs": {"mode": "file"}
	}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Skip("cannot write spec file")
	}
	defer func() { _ = os.Remove(specPath) }()

	// Log files don't exist → logs prints "File not found" and returns nil
	err = logs.Run([]string{"test-logs-proc"})
	if err != nil {
		t.Errorf("expected no error when log file missing, got %v", err)
	}
}

func TestRun_ExistingSpec_TailExistingLogs(t *testing.T) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine config dir")
	}
	specDir := filepath.Join(cfgDir, "lynx", "apps")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Skip("cannot create spec dir")
	}

	tmp := t.TempDir()
	specID := "test-logs-tail-0000-0000-0000-000000000001"
	// ResolveLogPaths joins logDir + specID + relative filenames, so create the
	// file at the resolved path to make the tail branch run.
	resolvedDir := filepath.Join(tmp, specID)
	if err := os.MkdirAll(resolvedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Generate enough lines so printLastNLines exercises the seek-then-read
	// branch (offset > 0 path).
	var bigStdout strings.Builder
	for i := 0; i < 500; i++ {
		bigStdout.WriteString("line-")
		bigStdout.WriteString(strconv.Itoa(i))
		bigStdout.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(resolvedDir, "out.log"), []byte(bigStdout.String()), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resolvedDir, "err.log"), []byte("ohno\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	specPath := filepath.Join(specDir, specID+".json")
	specContent := `{
		"version": 1,
		"id": "` + specID + `",
		"name": "test-logs-tail-proc",
		"namespace": "default",
		"exec": {"type": "command", "command": "echo"},
		"logs": {"mode": "file", "dir": "` + tmp + `", "stdout": "out.log", "stderr": "err.log"}
	}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o600); err != nil {
		t.Skip("cannot write spec file")
	}
	defer func() { _ = os.Remove(specPath) }()

	err = logs.Run([]string{"test-logs-tail-proc", "--lines", "10"})
	if err != nil {
		t.Errorf("expected no error tailing existing logs, got %v", err)
	}
}

func TestRun_AmbiguousMatch(t *testing.T) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no config dir")
	}
	specDir := filepath.Join(cfgDir, "lynx", "apps")
	if err := os.MkdirAll(specDir, 0o700); err != nil {
		t.Skip("cannot create spec dir")
	}

	for i := 0; i < 2; i++ {
		id := "test-logs-amb-" + strconv.Itoa(i) + "-0000-0000-000000000001"
		path := filepath.Join(specDir, id+".json")
		body := `{
			"version": 1,
			"id": "` + id + `",
			"name": "ambiguous",
			"namespace": "ns` + strconv.Itoa(i) + `",
			"exec": {"type": "command", "command": "echo"}
		}`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Skip("cannot write spec file")
		}
		defer func(p string) { _ = os.Remove(p) }(path)
	}

	err = logs.Run([]string{"ambiguous"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous argument") {
		t.Errorf("expected ambiguity error, got %v", err)
	}
}
