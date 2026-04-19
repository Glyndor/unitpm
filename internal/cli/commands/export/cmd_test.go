package export_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/export"
)

func setupSpecDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	return filepath.Join(tmp, "lynx", "apps")
}

func writeSpec(t *testing.T, dir, id, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRun_MissingNamespace(t *testing.T) {
	err := export.Run([]string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	if !strings.Contains(err.Error(), "export requires --namespace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_NamespaceNoValue(t *testing.T) {
	err := export.Run([]string{"--namespace"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_EmptyNamespaceString(t *testing.T) {
	setupSpecDir(t)
	err := export.Run([]string{"--namespace", ""})
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if !strings.Contains(err.Error(), "missing --namespace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Help(t *testing.T) {
	err := export.Run([]string{"--help"})
	if err != nil {
		t.Errorf("expected no error for --help, got %v", err)
	}
}

func TestRun_NoAppsInNamespace(t *testing.T) {
	setupSpecDir(t)
	// Empty spec dir → "no apps found"
	err := export.Run([]string{"--namespace", "prod"})
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
	if !strings.Contains(err.Error(), "no apps found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success_Command(t *testing.T) {
	dir := setupSpecDir(t)
	spec := `{
		"version": 1,
		"id": "aaa-111",
		"name": "api",
		"namespace": "prod",
		"cwd": "/tmp",
		"exec": {"type": "command", "command": "node", "args": ["server.js"]},
		"logs": {"mode": "file", "dir": "/var/log/lynx"},
		"restart": {"policy": "always", "max_retries": 5, "backoff_ms": 1000, "backoff_type": "expo", "stop_on_exit": [0]}
	}`
	writeSpec(t, dir, "aaa-111", spec)

	err := export.Run([]string{"--namespace", "prod"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_Success_Entry(t *testing.T) {
	dir := setupSpecDir(t)
	spec := `{
		"version": 1,
		"id": "bbb-222",
		"name": "worker",
		"namespace": "prod",
		"cwd": "/tmp",
		"exec": {"type": "entry", "entry": "index.js", "runtime": "node"}
	}`
	writeSpec(t, dir, "bbb-222", spec)

	err := export.Run([]string{"--namespace", "prod"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_FiltersByNamespace(t *testing.T) {
	dir := setupSpecDir(t)
	// 2 specs in different namespaces
	writeSpec(t, dir, "aaa-111",
		`{"version":1,"id":"aaa-111","name":"api","namespace":"prod",`+
			`"exec":{"type":"command","command":"echo"}}`)
	writeSpec(t, dir, "bbb-222",
		`{"version":1,"id":"bbb-222","name":"dev","namespace":"staging",`+
			`"exec":{"type":"command","command":"echo"}}`)

	// Export prod → only api
	err := export.Run([]string{"--namespace", "prod"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Export staging → only dev
	err = export.Run([]string{"--namespace", "staging"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Export nonexistent → error
	err = export.Run([]string{"--namespace", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestRun_ShortFlag(t *testing.T) {
	dir := setupSpecDir(t)
	writeSpec(t, dir, "aaa-111",
		`{"version":1,"id":"aaa-111","name":"api","namespace":"prod",`+
			`"exec":{"type":"command","command":"echo"}}`)

	err := export.Run([]string{"-n", "prod"})
	if err != nil {
		t.Fatalf("expected no error for -n flag, got %v", err)
	}
}

func TestGetSpec(t *testing.T) {
	spec := export.GetSpec()
	if spec.Name != "export" {
		t.Errorf("expected name 'export', got %s", spec.Name)
	}
}
