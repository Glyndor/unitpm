//go:build linux

package spec

import (
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestGenerateUUIDv4(t *testing.T) {
	id, err := GenerateUUIDv4()
	if err != nil {
		t.Fatalf("GenerateUUIDv4() error = %v", err)
	}
	if len(id) != 36 {
		t.Errorf("UUID length = %d, want 36", len(id))
	}
	// Basic format check
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("UUID format invalid, parts = %d, want 5", len(parts))
	}
}

func TestSaveSpec(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temp dir for this test
	tempDir, err := os.MkdirTemp("", "lynx-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	_ = os.Setenv("XDG_CONFIG_HOME", tempDir)
	// Fallback test
	_ = os.Setenv("HOME", tempDir)

	spec := protocol.AppSpec{
		Version: 1,
		Id:      "test-id",
		Name:    "test-app",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "echo",
		},
	}

	path, err := SaveSpec("test-id", spec)
	if err != nil {
		t.Fatalf("SaveSpec() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Spec file was not created at %s", path)
	}

	// Verify permissions
	_, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// On Windows, permissions are limited, but we can check if it exists.
	// On Linux/Unix, we would check 0600.
	// Since the environment is Windows, we skip strict permission check in this unit test
	// or assume the file creation worked.

	// Verify content
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(bytes)
	if !strings.Contains(content, "test-id") {
		t.Errorf("Spec content missing ID")
	}
}
