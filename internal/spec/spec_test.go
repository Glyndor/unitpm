package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestGetSpecDir(t *testing.T) {
	// Mock XDG_CONFIG_HOME
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir, err := GetSpecDir()
	if err != nil {
		t.Fatalf("GetSpecDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "lynx", "apps")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}

	// Verify directory exists and permissions
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Spec dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Spec dir is not a directory")
	}
	// Windows doesn't strictly enforce 0700 in the same way, so we skip permission check on Windows
	if os.PathSeparator == '/' {
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("Expected permissions 0700, got %o", perm)
		}
	}
}

func TestSaveLoadDeleteSpec(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	id := "test-app-id"
	spec := protocol.AppSpec{
		Name:      "test-app",
		Namespace: "test-ns",
		Exec: protocol.AppExec{
			Command: "echo hello",
		},
	}

	// Test SaveSpec
	path, err := SaveSpec(id, spec)
	if err != nil {
		t.Fatalf("SaveSpec failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Spec file not found at %s", path)
	}

	// Test LoadSpec
	loaded, err := LoadSpec(id)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}

	if loaded.Name != spec.Name {
		t.Errorf("Expected name %s, got %s", spec.Name, loaded.Name)
	}
	if loaded.Namespace != spec.Namespace {
		t.Errorf("Expected namespace %s, got %s", spec.Namespace, loaded.Namespace)
	}

	// Test LoadAll
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("Expected 1 spec, got %d", len(all))
	}
	if all[0].Name != spec.Name {
		t.Errorf("Expected name %s, got %s", spec.Name, all[0].Name)
	}

	// Test DeleteSpec
	if err := DeleteSpec(id); err != nil {
		t.Fatalf("DeleteSpec failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Spec file still exists after delete")
	}

	// Verify LoadSpec fails
	if _, err := LoadSpec(id); err == nil {
		t.Error("LoadSpec should fail after delete")
	}
}

func TestGenerateUUIDv4(t *testing.T) {
	uuid1, err := GenerateUUIDv4()
	if err != nil {
		t.Fatalf("GenerateUUIDv4 failed: %v", err)
	}
	if len(uuid1) == 0 {
		t.Error("UUID is empty")
	}

	uuid2, err := GenerateUUIDv4()
	if err != nil {
		t.Fatalf("GenerateUUIDv4 failed: %v", err)
	}

	if uuid1 == uuid2 {
		t.Error("Generated identical UUIDs")
	}
}
