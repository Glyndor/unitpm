//go:build linux

package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// newBannerTestProcess builds a Process configured to write logs to two
// per-test files so banner emission can be inspected. Caller is responsible
// for Start/Stop. setupTestEnv is registered for cleanup.
func newBannerTestProcess(
	t *testing.T,
	cmd string,
	args []string,
	restart *protocol.AppRestart,
) (*Process, string, string) {
	t.Helper()
	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "banner-test",
		Exec: protocol.AppExec{Type: "command", Command: cmd, Args: args},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: stdoutPath,
			Stderr: stderrPath,
		},
		Restart: restart,
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	return p, stdoutPath, stderrPath
}

// waitForMarker polls path until it contains marker or timeout fires.
// Returns final content for additional assertions.
func waitForMarker(t *testing.T, path, marker string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), marker) {
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("marker %q not seen in %s within %s. content=%q", marker, path, timeout, string(data))
	return ""
}

func TestBanner_StartStopWritesToBothStreams(t *testing.T) {
	p, stdoutPath, stderrPath := newBannerTestProcess(t, "sleep", []string{"30"}, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForMarker(t, stdoutPath, "STARTED", time.Second)
	waitForMarker(t, stderrPath, "STARTED", time.Second)

	if err := p.Stop(true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitForMarker(t, stdoutPath, "STOPPED", time.Second)
	waitForMarker(t, stderrPath, "STOPPED", time.Second)
}

func TestBanner_RestartSuppressesNested(t *testing.T) {
	p, stdoutPath, _ := newBannerTestProcess(t, "sleep", []string{"30"}, nil)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForMarker(t, stdoutPath, "STARTED", time.Second)

	if err := p.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitForMarker(t, stdoutPath, "RESTARTED", 2*time.Second)

	// Give the inner Start time to (potentially) fire — it should NOT emit
	// a second STARTED because inRestart is set.
	time.Sleep(300 * time.Millisecond)

	data, _ := os.ReadFile(stdoutPath)
	content := string(data)
	// "==  STARTED" is anchored — avoids matching the STARTED substring
	// inside "RESTARTED".
	if got := strings.Count(content, "==  STARTED"); got != 1 {
		t.Errorf("expected 1 STARTED (initial only), got %d. content=%q", got, content)
	}
	if strings.Contains(content, "==  STOPPED") {
		t.Errorf("Restart should not emit STOPPED, content=%q", content)
	}
	if strings.Contains(content, "==  EXITED") {
		t.Errorf("Restart should not emit EXITED, content=%q", content)
	}
	if strings.Contains(content, "AUTO-RESTART") {
		t.Errorf("user Restart should not race with handleRestart, content=%q", content)
	}

	_ = p.Stop(true)
}

func TestBanner_ExitedOnNaturalExit(t *testing.T) {
	restart := &protocol.AppRestart{Policy: "never"}
	p, stdoutPath, _ := newBannerTestProcess(t, "true", nil, restart)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForMarker(t, stdoutPath, "EXITED  code=0", 2*time.Second)
}

func TestBanner_AutoRestartFiresAfterFailure(t *testing.T) {
	restart := &protocol.AppRestart{
		Policy:      "on-failure",
		MaxRetries:  1,
		BackoffMs:   50,
		BackoffType: "linear",
	}
	p, stdoutPath, _ := newBannerTestProcess(t, "false", nil, restart)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForMarker(t, stdoutPath, "EXITED  code=1", 2*time.Second)
	waitForMarker(t, stdoutPath, "AUTO-RESTART  attempt=1", 2*time.Second)

	_ = p.Stop(true)
}

// TestBanner_CombinedLogDedupes covers the case where stdout and stderr
// resolve to the same path (combined log). emitBanner during running uses
// p.logFiles which holds only one *os.File, and emitBannerByPath
// (auto-restart path) dedupes via the seen map. Either bypass would cause
// two banner blocks per event in the file.
func TestBanner_CombinedLogDedupes(t *testing.T) {
	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	combined := filepath.Join(logDir, "combined.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "banner-combined",
		Exec: protocol.AppExec{Type: "command", Command: "sleep", Args: []string{"30"}},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: combined,
			Stderr: combined,
		},
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForMarker(t, combined, "STARTED", time.Second)

	if err := p.Stop(true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitForMarker(t, combined, "STOPPED", time.Second)

	data, _ := os.ReadFile(combined)
	content := string(data)
	if got := strings.Count(content, "==  STARTED"); got != 1 {
		t.Errorf("combined log: expected 1 STARTED, got %d. content=%q", got, content)
	}
	if got := strings.Count(content, "==  STOPPED"); got != 1 {
		t.Errorf("combined log: expected 1 STOPPED, got %d. content=%q", got, content)
	}
}

// TestBanner_AutoRestartCombinedLogDedupes exercises emitBannerByPath's
// dedupe on the failure path: cached p.stdoutPath == p.stderrPath, and the
// seen map must prevent the AUTO-RESTART block from being written twice.
func TestBanner_AutoRestartCombinedLogDedupes(t *testing.T) {
	restore := setupTestEnv(t)
	t.Cleanup(restore)

	id := uuid.Must(uuid.NewV7()).String()
	logDir := t.TempDir()
	combined := filepath.Join(logDir, "combined.log")

	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "banner-combined-auto",
		Exec: protocol.AppExec{Type: "command", Command: "false"},
		Logs: &protocol.AppLogs{
			Mode:   "file",
			Dir:    logDir,
			Stdout: combined,
			Stderr: combined,
		},
		Restart: &protocol.AppRestart{
			Policy:      "on-failure",
			MaxRetries:  1,
			BackoffMs:   50,
			BackoffType: "linear",
		},
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForMarker(t, combined, "AUTO-RESTART  attempt=1", 2*time.Second)
	_ = p.Stop(true)

	data, _ := os.ReadFile(combined)
	content := string(data)
	if got := strings.Count(content, "AUTO-RESTART  attempt=1"); got != 1 {
		t.Errorf("combined log: expected 1 AUTO-RESTART, got %d. content=%q", got, content)
	}
}

func TestBanner_NotEmittedInInheritMode(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.Must(uuid.NewV7()).String()
	spec := protocol.AppSpec{
		Version: 1, ID: id, Name: "banner-inherit",
		Exec: protocol.AppExec{Type: "command", Command: "true"},
		Logs: &protocol.AppLogs{Mode: "inherit"},
	}
	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	// Should not panic / error even though no log files are open.
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait briefly so monitor runs and emitBanner's no-op path executes.
	time.Sleep(200 * time.Millisecond)
	_ = p.Stop(true)
}
