//go:build linux

package execenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	// Create a temporary env file
	tmpDir, err := os.MkdirTemp("", "lynx-test-env")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	envPath := filepath.Join(tmpDir, "env")
	content := []byte(`
# This is a comment
KEY1=value1
KEY2=value2 with spaces
KEY3=value3
EMPTY=
# COMMENTed=ignored
`)
	if err := os.WriteFile(envPath, content, 0600); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// Helper to check env
	checkEnv := func(key, expected string) {
		val := os.Getenv(key)
		if val != expected {
			t.Errorf("expected %s=%q, got %q", key, expected, val)
		}
	}

	// Clean up envs after test
	defer func() {
		os.Unsetenv("KEY1")
		os.Unsetenv("KEY2")
		os.Unsetenv("KEY3")
		os.Unsetenv("EMPTY")
	}()

	// Run loadEnv
	if err := loadEnv(envPath); err != nil {
		t.Fatalf("loadEnv failed: %v", err)
	}

	// Verify
	checkEnv("KEY1", "value1")
	checkEnv("KEY2", "value2 with spaces")
	checkEnv("KEY3", "value3")
	checkEnv("EMPTY", "")
}
