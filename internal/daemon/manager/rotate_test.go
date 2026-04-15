package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateIfLarge(t *testing.T) {
	tmp := t.TempDir()
	log := filepath.Join(tmp, "stdout.log")

	// Override knobs for the test
	oldBytes, oldKeep := rotateMaxBytes, rotateKeep
	rotateMaxBytes = 20
	rotateKeep = 2
	t.Cleanup(func() { rotateMaxBytes, rotateKeep = oldBytes, oldKeep })

	// Below threshold — no rotation
	if err := os.WriteFile(log, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLarge(log)
	if _, err := os.Stat(log + ".1"); err == nil {
		t.Error("unexpected .1 created on small file")
	}

	// Above threshold — rotates
	big := strings.Repeat("x", 30)
	if err := os.WriteFile(log, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLarge(log)
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Errorf("original log should be gone, got %v", err)
	}
	if b, err := os.ReadFile(log + ".1"); err != nil || string(b) != big {
		t.Errorf(".1 missing or wrong: %v, %q", err, string(b))
	}

	// Second rotation: .1 -> .2
	if err := os.WriteFile(log, []byte(big+"2"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLarge(log)
	if b, _ := os.ReadFile(log + ".2"); string(b) != big {
		t.Errorf(".2 expected old content, got %q", string(b))
	}
	if b, _ := os.ReadFile(log + ".1"); string(b) != big+"2" {
		t.Errorf(".1 expected newer content, got %q", string(b))
	}

	// Third rotation: oldest (.2) deleted, .1 -> .2, new -> .1
	if err := os.WriteFile(log, []byte(big+"3"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLarge(log)
	if _, err := os.Stat(log + ".3"); err == nil {
		t.Error(".3 should never exist with keep=2")
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("LYNX_TEST_INT", "42")
	if v := envInt("LYNX_TEST_INT", 99); v != 42 {
		t.Errorf("got %d want 42", v)
	}
	if v := envInt("LYNX_TEST_INT_MISSING", 99); v != 99 {
		t.Errorf("got %d want 99", v)
	}
	t.Setenv("LYNX_TEST_INT_BAD", "nope")
	if v := envInt("LYNX_TEST_INT_BAD", 99); v != 99 {
		t.Errorf("got %d want fallback", v)
	}
}
