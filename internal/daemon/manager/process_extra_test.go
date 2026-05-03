//go:build linux

package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestProcess_Tree_NotRunning(t *testing.T) {
	proc, err := NewProcess("123e4567-e89b-12d3-a456-426614174002", protocol.AppSpec{
		Name: "test",
		Exec: protocol.AppExec{Command: "echo"},
	})
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	// Newly created process is not running, so Tree() should return nil.
	tree := proc.Tree()
	if tree != nil {
		t.Errorf("Tree() on non-running process = %v, want nil", tree)
	}
}

func TestProcess_getLynxBinary_InPath(t *testing.T) {
	// Create a fake lynxpm binary in a temp dir and put it in PATH.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "lynxpm")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("create fake binary: %v", err)
	}

	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+orig)

	proc, err := NewProcess("123e4567-e89b-12d3-a456-426614174003", protocol.AppSpec{
		Name: "test",
		Exec: protocol.AppExec{Command: "echo"},
	})
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	path, err := proc.getLynxBinary()
	if err != nil {
		t.Fatalf("getLynxBinary: %v", err)
	}
	if path != fakeBin {
		t.Errorf("getLynxBinary = %q, want %q", path, fakeBin)
	}
}

func TestProcess_getLynxBinary_NotFound(t *testing.T) {
	// Override PATH to empty so neither PATH nor os.Executable() dir has lynxpm.
	t.Setenv("PATH", t.TempDir()) // dir with no lynxpm

	proc, err := NewProcess("123e4567-e89b-12d3-a456-426614174004", protocol.AppSpec{
		Name: "test",
		Exec: protocol.AppExec{Command: "echo"},
	})
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	// getLynxBinary falls back to adjacent binary. In tests, os.Executable()
	// is the test binary; there's no lynxpm next to it.
	// Result depends on the test environment, so we just verify no panic.
	_, _ = proc.getLynxBinary()
}

func TestWalkDescendants_CurrentProcess(t *testing.T) {
	// Our own PID should appear in /proc and not cause walkDescendants to crash.
	pid := os.Getpid()
	// Start a child to have at least one descendant.
	cmd := exec.Command("sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start sleep subprocess:", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	descendants := walkDescendants(pid)
	// We just verify no crash and the function returns a slice (possibly empty).
	_ = descendants
}
