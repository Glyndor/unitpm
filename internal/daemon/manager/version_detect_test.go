package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectVersion_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app","version":"2.1.0"}`), 0600)

	v := detectProjectVersion(dir)
	if v != "2.1.0" {
		t.Errorf("expected 2.1.0, got %q", v)
	}
}

func TestDetectProjectVersion_CargoToml(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"app\"\nversion = \"0.3.5\"\n"), 0600)

	v := detectProjectVersion(dir)
	if v != "0.3.5" {
		t.Errorf("expected 0.3.5, got %q", v)
	}
}

func TestDetectProjectVersion_PyprojectToml(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"app\"\nversion = \"1.2.3\"\n"), 0600)

	v := detectProjectVersion(dir)
	if v != "1.2.3" {
		t.Errorf("expected 1.2.3, got %q", v)
	}
}

func TestDetectProjectVersion_SetupCfg(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "setup.cfg"), []byte("[metadata]\nname = app\nversion = 4.0.0\n"), 0600)

	v := detectProjectVersion(dir)
	if v != "4.0.0" {
		t.Errorf("expected 4.0.0, got %q", v)
	}
}

func TestDetectProjectVersion_Priority(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"1.0.0"}`), 0600)
	_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("version = \"2.0.0\"\n"), 0600)

	v := detectProjectVersion(dir)
	if v != "1.0.0" {
		t.Errorf("package.json should take priority, got %q", v)
	}
}

func TestDetectProjectVersion_NoFiles(t *testing.T) {
	dir := t.TempDir()
	v := detectProjectVersion(dir)
	if v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}

func TestDetectProjectVersion_EmptyCwd(t *testing.T) {
	v := detectProjectVersion("")
	if v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}
