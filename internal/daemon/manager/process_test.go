//go:build linux

package manager

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestPrepareEnv(t *testing.T) {
	// Mock spec
	spec := protocol.AppSpec{
		Name: "test-app",
		Exec: protocol.AppExec{
			Command: "echo",
		},
	}

	// Case 1: Dynamic Mode (Should not have HOME)
	dynamicSpec := spec
	dynamicSpec.RunAs = &protocol.RunAsPolicy{Mode: "dynamic"}

	proc, err := NewProcess("123e4567-e89b-12d3-a456-426614174000", dynamicSpec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	// Inject a fake HOME into process env for testing logic (mocking os.Environ is hard without refactor,
	// but we can rely on the fact that 'go test' runs with some env)
	// We just check that the result does NOT contain HOME if it was in os.Environ()
	// Actually, in system mode (uid 0), we whitelist.
	// Since we can't easily mock Getuid(), we test the logic branches by
	// inference or assuming we are user mode (uid != 0).
	// If we are user mode, os.Environ() is used. It likely has HOME.

	env, err := proc.prepareEnv()
	if err != nil {
		t.Fatalf("prepareEnv failed: %v", err)
	}

	for _, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			t.Errorf("Dynamic mode should not have HOME, found: %s", e)
		}
	}

	// Case 2: Normal Mode (Should have HOME)
	normalSpec := spec
	proc2, err := NewProcess("123e4567-e89b-12d3-a456-426614174001", normalSpec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}
	env2, err := proc2.prepareEnv()
	if err != nil {
		t.Fatalf("prepareEnv failed: %v", err)
	}

	foundHome := false
	for _, e := range env2 {
		if strings.HasPrefix(e, "HOME=") {
			foundHome = true
			break
		}
	}
	if !foundHome {
		t.Error("Normal mode should have HOME")
	}
}
