package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLog_Writes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l := Open(path)
	l.Log(Event{Action: "start", UID: "1000", Target: "abc", Name: "api", Success: true})
	l.Log(Event{Action: "delete", UID: "1000", Target: "xyz", Success: false, Error: "not found"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	var e1, e2 Event
	if err := json.Unmarshal([]byte(lines[0]), &e1); err != nil {
		t.Fatalf("line 1: %v", err)
	}
	if e1.Action != "start" || !e1.Success || e1.Time == "" {
		t.Errorf("unexpected event: %+v", e1)
	}
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatalf("line 2: %v", err)
	}
	if e2.Success || e2.Error != "not found" {
		t.Errorf("unexpected event: %+v", e2)
	}
}

func TestOpen_BadPath_ReturnsDisabled(t *testing.T) {
	l := Open("/proc/nonwritable/audit.log")
	if l != disabled {
		t.Error("expected Disabled() sentinel on open failure")
	}
}

func TestDisabled_LogIsNoOp(t *testing.T) {
	l := Disabled()
	l.Log(Event{Action: "x"})
}

func TestOpen_FileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l := Open(path)
	l.Log(Event{Action: "start", Success: true})
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 perms, got %o", fi.Mode().Perm())
	}
}
