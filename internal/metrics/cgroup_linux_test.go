//go:build linux

package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCPUUsage_Parses(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cpu.stat")
	contents := "usage_usec 12345\nuser_usec 10000\nsystem_usec 2345\n"
	if err := os.WriteFile(tmp, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readCPUUsage(tmp)
	if err != nil {
		t.Fatalf("readCPUUsage: %v", err)
	}
	if got != 12345 {
		t.Errorf("got=%d want 12345", got)
	}
}

func TestReadCPUUsage_MissingField(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cpu.stat")
	if err := os.WriteFile(tmp, []byte("user_usec 10000\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := readCPUUsage(tmp); err == nil {
		t.Error("expected error when usage_usec missing")
	}
}

func TestReadCPUUsage_FileMissing(t *testing.T) {
	if _, err := readCPUUsage(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadCPUUsage_BadValue(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cpu.stat")
	if err := os.WriteFile(tmp, []byte("usage_usec abc\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := readCPUUsage(tmp); err == nil {
		t.Error("expected parse error")
	}
}

func TestGetCgroupPath_Self(t *testing.T) {
	if _, err := os.Stat("/proc/self/cgroup"); os.IsNotExist(err) {
		t.Skip("no /proc/self/cgroup")
	}
	p, err := getCgroupPath(os.Getpid())
	if err != nil {
		t.Skipf("no v2 cgroup for self: %v", err)
	}
	if p == "" {
		t.Error("empty cgroup path")
	}
}

func TestGetCgroupPath_BadPid(t *testing.T) {
	if _, err := getCgroupPath(2147483646); err == nil {
		t.Error("expected error for nonexistent pid")
	}
}

func TestNewCgroupCollector_NoV2(t *testing.T) {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		t.Skip("v2 is mounted; skip negative case")
	}
	if _, err := NewCgroupCollector(os.Getpid()); err == nil {
		t.Error("expected error when v2 unavailable")
	}
}

func TestCgroupCollector_CollectAndDelta(t *testing.T) {
	c, err := NewCgroupCollector(os.Getpid())
	if err != nil {
		t.Skipf("cgroup v2 not usable: %v", err)
	}
	first, err := c.Collect()
	if err != nil {
		t.Fatalf("first collect: %v", err)
	}
	if first.MemoryBytes <= 0 {
		t.Errorf("memory should be > 0, got %d", first.MemoryBytes)
	}
	// Second collect should compute a CPU% (may be 0.0 but no error).
	if _, err := c.Collect(); err != nil {
		t.Fatalf("second collect: %v", err)
	}
}
