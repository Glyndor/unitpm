//go:build linux

package manager_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// newTestManager returns a Manager backed by a temp XDG dir. The returned
// cleanup function removes the temp dir.
func newTestManager(t *testing.T) *manager.Manager {
	t.Helper()

	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "lynx", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", tempDir)
	t.Setenv("HOME", tempDir)

	return manager.NewManager()
}

// quickSpec returns a spec running `sleep 10` with the given name + namespace.
func quickSpec(t *testing.T, name, namespace string) protocol.AppSpec {
	t.Helper()
	return protocol.AppSpec{
		Version:   1,
		ID:        uuid.Must(uuid.NewV7()).String(),
		Name:      name,
		Namespace: namespace,
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"10"},
		},
	}
}

func TestManager_Get_Exists(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "getme", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	if _, ok := mgr.Get(s.ID); !ok {
		t.Error("Get(existing ID) returned !ok")
	}
}

func TestManager_Get_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	if _, ok := mgr.Get("nonexistent-id"); ok {
		t.Error("Get(nonexistent) returned ok=true")
	}
}

func TestManager_Stop_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Stop("nope")
	if err == nil {
		t.Fatal("expected error for Stop(nonexistent)")
	}
	if !strings.Contains(err.Error(), "process not found") {
		t.Errorf("expected 'process not found' error, got: %v", err)
	}
}

func TestManager_Delete_RemovesFromManager(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "delete-me", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}

	if err := mgr.Delete(s.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, ok := mgr.Get(s.ID); ok {
		t.Error("process still present in manager after Delete")
	}
}

func TestManager_Delete_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Delete("nope")
	if err == nil {
		t.Fatal("expected error for Delete(nonexistent)")
	}
}

func TestManager_Restart_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Restart("nope")
	if err == nil {
		t.Fatal("expected error for Restart(nonexistent)")
	}
}

func TestManager_Reset_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Reset("nope")
	if err == nil {
		t.Fatal("expected error for Reset(nonexistent)")
	}
}

func TestManager_Reset_NoError_OnExisting(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "resetme", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	if err := mgr.Reset(s.ID); err != nil {
		t.Errorf("Reset returned error on existing process: %v", err)
	}
}

func TestManager_ResolveID_ExactID(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "exact", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	got, err := mgr.ResolveID(s.ID)
	if err != nil {
		t.Fatalf("ResolveID(%s) failed: %v", s.ID, err)
	}
	if got != s.ID {
		t.Errorf("got %q want %q", got, s.ID)
	}
}

func TestManager_ResolveID_Prefix(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "prefix", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	// Use first 8 chars of UUID as prefix (unique with 1 process).
	prefix := s.ID[:8]
	got, err := mgr.ResolveID(prefix)
	if err != nil {
		t.Fatalf("ResolveID(%s) failed: %v", prefix, err)
	}
	if got != s.ID {
		t.Errorf("got %q want %q", got, s.ID)
	}
}

func TestManager_ResolveID_Name(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "uniquename", "default")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	got, err := mgr.ResolveID("uniquename")
	if err != nil {
		t.Fatalf("ResolveID(name) failed: %v", err)
	}
	if got != s.ID {
		t.Errorf("got %q want %q", got, s.ID)
	}
}

func TestManager_ResolveID_NamespaceName(t *testing.T) {
	mgr := newTestManager(t)

	s := quickSpec(t, "api", "prod")
	if _, err := mgr.StartWithSpec(s); err != nil {
		t.Fatalf("StartWithSpec failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s.ID) }()

	got, err := mgr.ResolveID("prod:api")
	if err != nil {
		t.Fatalf("ResolveID(prod:api) failed: %v", err)
	}
	if got != s.ID {
		t.Errorf("got %q want %q", got, s.ID)
	}
}

func TestManager_ResolveID_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.ResolveID("does-not-exist")
	if err == nil {
		t.Fatal("expected error for ResolveID(unknown)")
	}
	if !strings.Contains(err.Error(), "process not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_ResolveID_AmbiguousName(t *testing.T) {
	mgr := newTestManager(t)

	// Two processes with same name in different namespaces — bare name
	// lookup should be ambiguous.
	s1 := quickSpec(t, "dup", "ns1")
	s2 := quickSpec(t, "dup", "ns2")
	if _, err := mgr.StartWithSpec(s1); err != nil {
		t.Fatalf("StartWithSpec(s1) failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s1.ID) }()
	if _, err := mgr.StartWithSpec(s2); err != nil {
		t.Fatalf("StartWithSpec(s2) failed: %v", err)
	}
	defer func() { _ = mgr.Stop(s2.ID) }()

	_, err := mgr.ResolveID("dup")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' in error, got: %v", err)
	}
}

func TestManager_List_Empty(t *testing.T) {
	mgr := newTestManager(t)
	got := mgr.List()
	if len(got) != 0 {
		t.Errorf("List on empty manager returned %d entries, want 0", len(got))
	}
}

func TestManager_List_ReturnsAll(t *testing.T) {
	mgr := newTestManager(t)

	specs := []protocol.AppSpec{
		quickSpec(t, "one", "default"),
		quickSpec(t, "two", "default"),
		quickSpec(t, "three", "default"),
	}
	for _, s := range specs {
		if _, err := mgr.StartWithSpec(s); err != nil {
			t.Fatalf("StartWithSpec(%s) failed: %v", s.Name, err)
		}
		defer func(id string) { _ = mgr.Stop(id) }(s.ID)
	}

	got := mgr.List()
	if len(got) != len(specs) {
		t.Errorf("List returned %d entries, want %d", len(got), len(specs))
	}
}

func TestManager_Scale_InvalidTarget(t *testing.T) {
	mgr := newTestManager(t)

	tests := []struct {
		name   string
		target int
	}{
		{"negative", -1},
		{"too large", 1025},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mgr.Scale("default", "foo", tc.target)
			if err == nil {
				t.Fatalf("Scale(target=%d) expected error", tc.target)
			}
		})
	}
}

func TestManager_Scale_NoTemplate(t *testing.T) {
	mgr := newTestManager(t)
	// No process with name "ghost" exists → cannot scale up without template.
	_, err := mgr.Scale("default", "ghost", 3)
	if err == nil {
		t.Fatal("expected error scaling with no template")
	}
}

func TestManager_Reload_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	// No spec on disk and no process with this ID.
	err := mgr.Reload("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for Reload(nonexistent)")
	}
}
