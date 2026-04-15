//go:build linux

package execenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	tmpDir := t.TempDir()

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

	checkEnv := func(key, expected string) {
		val := os.Getenv(key)
		if val != expected {
			t.Errorf("expected %s=%q, got %q", key, expected, val)
		}
	}

	defer func() {
		os.Unsetenv("KEY1")
		os.Unsetenv("KEY2")
		os.Unsetenv("KEY3")
		os.Unsetenv("EMPTY")
	}()

	if err := loadEnv(envPath); err != nil {
		t.Fatalf("loadEnv failed: %v", err)
	}

	checkEnv("KEY1", "value1")
	checkEnv("KEY2", "value2 with spaces")
	checkEnv("KEY3", "value3")
	checkEnv("EMPTY", "")
}

func TestLoadEnv_MissingFile(t *testing.T) {
	err := loadEnv("/nonexistent/path/env")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRun_NoArgs(t *testing.T) {
	err := Run([]string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	err := Run([]string{"this-binary-absolutely-does-not-exist-xyz-123"})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_BadCredentialsDir(t *testing.T) {
	// Set CREDENTIALS_DIRECTORY to a path that doesn't exist → loadEnv warns but Run continues
	// Then use a missing command so Run errors out before syscall.Exec
	t.Setenv("CREDENTIALS_DIRECTORY", "/nonexistent/creds/dir")
	err := Run([]string{"this-binary-absolutely-does-not-exist-xyz-123"})
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should come from command lookup, not creds loading
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := GetSpec()
	if spec.Name != "_exec-env" {
		t.Errorf("expected name '_exec-env', got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Error("expected non-empty description")
	}
}
