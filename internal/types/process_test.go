package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProcessStateConstants(t *testing.T) {
	cases := map[ProcessState]string{
		StateRunning:    "running",
		StateOnline:     "online",
		StateStopped:    "stopped",
		StateFailed:     "failed",
		StateExited:     "exited",
		StateRestarting: "restarting",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("ProcessState %q != %q", got, want)
		}
	}
	if DefaultNamespace != "default" {
		t.Errorf("DefaultNamespace=%q want default", DefaultNamespace)
	}
}

func TestProcessInfoMarshalRoundTrip(t *testing.T) {
	in := ProcessInfo{
		ID: "p1", Name: "api", Namespace: "ns", Version: "1.0", Mode: "fork",
		PID: 1234, Uptime: 5000, Restarts: 2, State: StateOnline,
		CPU: 12.5, Memory: 1024, User: "root", Watch: true,
		GitBranch: "main", GitCommit: "abc", GitDirty: true, CreatedAt: "2024-01-01",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ProcessInfo
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("roundtrip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

func TestProcessInfoOmitEmpty(t *testing.T) {
	b, err := json.Marshal(ProcessInfo{ID: "p", State: StateRunning})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, k := range []string{"git_branch", "git_commit", "git_dirty", "created_at"} {
		if strings.Contains(s, k) {
			t.Errorf("expected %q omitted, got %s", k, s)
		}
	}
	for _, k := range []string{"id", "pid", "uptime_ms", "memory_bytes"} {
		if !strings.Contains(s, k) {
			t.Errorf("expected %q present, got %s", k, s)
		}
	}
}
