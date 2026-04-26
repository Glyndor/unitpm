//go:build linux

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setRootEuid(t *testing.T) {
	t.Helper()
	prev := currentEuid
	currentEuid = func() int { return 0 }
	t.Cleanup(func() { currentEuid = prev })
}

func TestGetLogDir_RootDefault(t *testing.T) {
	setRootEuid(t)
	dir, err := GetLogDir("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if dir != LogRoot {
		t.Errorf("dir=%q want %q", dir, LogRoot)
	}
}

func TestResolveRootLogDir_NotAbsolute(t *testing.T) {
	setRootEuid(t)
	_, err := GetLogDir("relative/path")
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("want absolute error, got %v", err)
	}
}

func TestResolveRootLogDir_OutsideAllowedRoots(t *testing.T) {
	setRootEuid(t)
	_, err := GetLogDir("/etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "outside allowed") {
		t.Errorf("want outside roots error, got %v", err)
	}
}

func TestResolveRootLogDir_WithinXDGStateHome(t *testing.T) {
	setRootEuid(t)
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	candidate := filepath.Join(tmp, "lynx/logs/sub")
	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := GetLogDir(candidate)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != candidate {
		t.Errorf("got %q want %q", got, candidate)
	}
}

func TestResolveRootLogDir_NonexistentInsideRoot(t *testing.T) {
	setRootEuid(t)
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	candidate := filepath.Join(tmp, "lynx/logs/does-not-exist")
	got, err := GetLogDir(candidate)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != candidate {
		t.Errorf("got %q want %q", got, candidate)
	}
}

func TestPathContainsUnsafeSymlink_Safe(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if pathContainsUnsafeSymlink(root, sub) {
		t.Error("expected safe path")
	}
}

func TestPathContainsUnsafeSymlink_EscapingSymlink(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	outside := t.TempDir()
	outsideResolved, _ := filepath.EvalSymlinks(outside)
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outsideResolved, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if !pathContainsUnsafeSymlink(root, filepath.Join(link, "x")) {
		t.Error("expected unsafe symlink detected")
	}
}

func TestMatchResolvedRoot_NonexistentSafe(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	candidate := filepath.Join(root, "fresh")
	if !matchResolvedRoot(root, candidate) {
		t.Error("expected match for nonexistent inside root")
	}
}

func TestMatchResolvedRoot_OutsideRoot(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if matchResolvedRoot(root, "/etc") {
		t.Error("expected /etc not to match root")
	}
}
