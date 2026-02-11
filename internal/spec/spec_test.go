//go:build linux

package spec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/google/uuid"
)

func TestGenerateUUIDv4(t *testing.T) {
	id, err := spec.GenerateUUIDv4()
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

	specData := protocol.AppSpec{
		Version: 1,
		ID:      "test-id",
		Name:    "test-app",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "echo",
		},
	}

	path, err := spec.SaveSpec("test-id", specData)
	if err != nil {
		t.Fatalf("SaveSpec() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Spec file was not created at %s", path)
	}

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

func TestLoadAll(t *testing.T) {
	// Setup temp XDG_CONFIG_HOME
	tempDir, err := os.MkdirTemp("", "lynx-spec-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer os.Unsetenv("XDG_CONFIG_HOME")

	// Create valid spec
	validID := uuid.NewString()
	validSpec := protocol.AppSpec{
		Version: 1,
		ID:      validID,
		Name:    "valid-app",
		Exec:    protocol.AppExec{Type: "command", Command: "true"},
	}
	if _, err := spec.SaveSpec(validID, validSpec); err != nil {
		t.Fatalf("failed to save valid spec: %v", err)
	}

	// Create disabled spec
	disabledID := uuid.NewString()
	disabledSpec := protocol.AppSpec{
		Version:  1,
		ID:       disabledID,
		Name:     "disabled-app",
		Exec:     protocol.AppExec{Type: "command", Command: "true"},
		Disabled: true,
	}
	if _, err := spec.SaveSpec(disabledID, disabledSpec); err != nil {
		t.Fatalf("failed to save disabled spec: %v", err)
	}

	// Create corrupted file
	specDir, _ := spec.GetSpecDir()
	corruptedPath := filepath.Join(specDir, "corrupted.json")
	if err := os.WriteFile(corruptedPath, []byte("{invalid-json"), 0600); err != nil {
		t.Fatalf("failed to write corrupted spec: %v", err)
	}

	// Create non-json file
	ignoredPath := filepath.Join(specDir, "readme.txt")
	if err := os.WriteFile(ignoredPath, []byte("ignore me"), 0600); err != nil {
		t.Fatalf("failed to write ignored file: %v", err)
	}

	// Test LoadAll
	loaded, err := spec.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() failed: %v", err)
	}

	// Verify results
	// Should load valid and disabled specs, ignore corrupted and non-json
	if len(loaded) != 2 {
		t.Errorf("LoadAll() returned %d specs, expected 2", len(loaded))
	}

	foundValid := false
	foundDisabled := false
	for _, s := range loaded {
		if s.ID == validID {
			foundValid = true
			if s.Disabled {
				t.Error("Valid spec loaded as disabled")
			}
		}
		if s.ID == disabledID {
			foundDisabled = true
			if !s.Disabled {
				t.Error("Disabled spec loaded as enabled")
			}
		}
	}

	if !foundValid {
		t.Error("Valid spec not found in loaded specs")
	}
	if !foundDisabled {
		t.Error("Disabled spec not found in loaded specs")
	}
}
