package manager

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestRotate_DelayCompress_FirstRotation pins logrotate's `delaycompress`
// semantics on the very first rotation: current → .1 (plain), no .gz
// archive yet. Compression only happens on the *next* cycle.
func TestRotate_DelayCompress_FirstRotation(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	cfg := rotateConfig{maxBytes: 20, keep: 12, delayCompress: true, notifEmpty: true}

	if err := os.WriteFile(logPath, []byte(strings.Repeat("a", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateNowCfg(logPath, cfg, time.Time{})

	if data, err := os.ReadFile(logPath + ".1"); err != nil || string(data) != strings.Repeat("a", 30) {
		t.Errorf(".1 should hold the plain pre-rotation content: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(logPath + ".1.gz"); !os.IsNotExist(err) {
		t.Errorf(".1.gz should not exist on first rotation with delaycompress: err=%v", err)
	}
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat current: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("current truncated, size=%d", info.Size())
	}
}

// TestRotate_DelayCompress_ChainGrowsCorrectly walks two rotations and
// verifies the chain matches `delaycompress`: most recent stays plain
// at .1, the previous .1 is compressed into .2.gz on the second cycle.
// Older .gz entries shift up by one slot each rotation.
func TestRotate_DelayCompress_ChainGrowsCorrectly(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	cfg := rotateConfig{maxBytes: 20, keep: 12, delayCompress: true, notifEmpty: true}

	// Cycle 1
	if err := os.WriteFile(logPath, []byte(strings.Repeat("a", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateNowCfg(logPath, cfg, time.Time{})

	// Cycle 2: writes a different payload; old .1 must move to .2.gz.
	if err := os.WriteFile(logPath, []byte(strings.Repeat("b", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateNowCfg(logPath, cfg, time.Time{})

	if data, err := os.ReadFile(logPath + ".1"); err != nil || string(data) != strings.Repeat("b", 30) {
		t.Errorf(".1 should hold the most recent cycle: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(logPath + ".1.gz"); !os.IsNotExist(err) {
		t.Errorf(".1.gz should not exist: err=%v", err)
	}
	if got := readGz(t, logPath+".2.gz"); got != strings.Repeat("a", 30) {
		t.Errorf(".2.gz should hold the compressed previous cycle, got %q", got)
	}
	// Cycle 3: plain .1 cycles into .2.gz, old .2.gz becomes .3.gz.
	if err := os.WriteFile(logPath, []byte(strings.Repeat("c", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateNowCfg(logPath, cfg, time.Time{})

	if data, _ := os.ReadFile(logPath + ".1"); string(data) != strings.Repeat("c", 30) {
		t.Errorf(".1 mismatch after cycle 3: %q", data)
	}
	if got := readGz(t, logPath+".2.gz"); got != strings.Repeat("b", 30) {
		t.Errorf(".2.gz mismatch after cycle 3: %q", got)
	}
	if got := readGz(t, logPath+".3.gz"); got != strings.Repeat("a", 30) {
		t.Errorf(".3.gz mismatch after cycle 3: %q", got)
	}
}

// TestRotate_NotifEmpty_SkipsZeroByteFile mirrors logrotate's
// `notifempty`: a 0-byte log is left alone. Without this guard the
// daemon would create endless empty .1 plain files on each tick.
func TestRotate_NotifEmpty_SkipsZeroByteFile(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := rotateConfig{maxBytes: 20, keep: 12, delayCompress: true, notifEmpty: true}

	if rotated := rotateNowCfg(logPath, cfg, time.Time{}); rotated {
		t.Error("rotation must be skipped on empty file when notifEmpty is set")
	}
	for _, suffix := range []string{".1", ".1.gz", ".2.gz"} {
		if _, err := os.Stat(logPath + suffix); !os.IsNotExist(err) {
			t.Errorf("%s should not exist after notifEmpty skip", suffix)
		}
	}
}

// TestRotate_AgeTrigger_Fires reproduces the weekly-style trigger: file
// is below the size threshold, but lastRotateAt is older than maxAge.
// rotation must happen anyway, otherwise idle-but-aging logs would
// never roll over.
func TestRotate_AgeTrigger_Fires(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	if err := os.WriteFile(logPath, []byte("not big yet"), 0o600); err != nil {
		t.Fatal(err)
	}
	// maxBytes far above current size, maxAge below time-since-anchor.
	cfg := rotateConfig{
		maxBytes:      1 << 30,
		keep:          12,
		maxAge:        50 * time.Millisecond,
		delayCompress: true,
		notifEmpty:    true,
	}
	anchor := time.Now().Add(-1 * time.Second)

	if !rotateNowCfg(logPath, cfg, anchor) {
		t.Fatal("expected age-based rotation, got no-op")
	}
	if data, err := os.ReadFile(logPath + ".1"); err != nil || string(data) != "not big yet" {
		t.Errorf("age-rotation did not preserve content into .1: data=%q err=%v", data, err)
	}
}

// TestRotate_AgeTrigger_HoldsBackWhenAnchorRecent guards the inverse:
// if lastRotateAt is fresh (e.g. just rotated), neither size nor age
// triggers fire. Prevents storms of consecutive rotations from a tight
// ticker.
func TestRotate_AgeTrigger_HoldsBackWhenAnchorRecent(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "stdout.log")
	if err := os.WriteFile(logPath, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := rotateConfig{maxBytes: 1 << 30, keep: 12, maxAge: 1 * time.Hour, delayCompress: true, notifEmpty: true}

	if rotateNowCfg(logPath, cfg, time.Now()) {
		t.Error("recent anchor + small file should not trigger rotation")
	}
}
