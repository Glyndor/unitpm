package manager

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readGz(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader %s: %v", path, err)
	}
	defer func() { _ = gr.Close() }()
	b, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestRotateIfLarge(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	cfg := rotateConfig{maxBytes: 20, keep: 2}

	// Below threshold — no rotation
	if err := os.WriteFile(logPath, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLargeCfg(logPath, cfg)
	if _, err := os.Stat(logPath + ".1.gz"); err == nil {
		t.Error("unexpected .1.gz created on small file")
	}

	// Above threshold — rotates and compresses
	big := strings.Repeat("x", 30)
	if err := os.WriteFile(logPath, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLargeCfg(logPath, cfg)

	// Original should be truncated (not deleted — open FDs keep working)
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("original log should still exist: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("original log should be truncated, got size %d", info.Size())
	}

	if content := readGz(t, logPath+".1.gz"); content != big {
		t.Errorf(".1.gz wrong content: got %q", content)
	}

	// Second rotation: .1.gz -> .2.gz
	big2 := big + "2"
	if err := os.WriteFile(logPath, []byte(big2), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLargeCfg(logPath, cfg)
	if content := readGz(t, logPath+".2.gz"); content != big {
		t.Errorf(".2.gz expected old content, got %q", content)
	}
	if content := readGz(t, logPath+".1.gz"); content != big2 {
		t.Errorf(".1.gz expected newer content, got %q", content)
	}

	// Third rotation: oldest (.2.gz) deleted, keep=2
	big3 := big + "3"
	if err := os.WriteFile(logPath, []byte(big3), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateIfLargeCfg(logPath, cfg)
	if _, err := os.Stat(logPath + ".3.gz"); err == nil {
		t.Error(".3.gz should never exist with keep=2")
	}
}
