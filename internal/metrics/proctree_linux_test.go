//go:build linux

package metrics_test

import (
	"os"
	"testing"

	"github.com/Jaro-c/Lynx/internal/metrics"
)

func BenchmarkProcTreeCollector(b *testing.B) {
	collector, err := metrics.NewProcTreeCollector(os.Getpid())
	if err != nil {
		b.Fatalf("failed to create collector: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := collector.Collect()
		if err != nil {
			b.Fatalf("failed to collect: %v", err)
		}
	}
}

func TestGetProcessTree_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	tree, err := metrics.GetProcessTree(pid)
	if err != nil {
		t.Fatalf("GetProcessTree(%d): %v", pid, err)
	}
	if len(tree) == 0 {
		t.Fatal("expected at least one entry (the process itself)")
	}
	root := tree[0]
	if root.PID != pid {
		t.Errorf("root PID = %d, want %d", root.PID, pid)
	}
	if root.Depth != 0 {
		t.Errorf("root depth = %d, want 0", root.Depth)
	}
	if root.Comm == "" {
		t.Error("root Comm is empty")
	}
	if root.MemoryBytes <= 0 {
		t.Errorf("root MemoryBytes = %d, want > 0", root.MemoryBytes)
	}
}

func TestGetProcessTree_DepthsNonNegative(t *testing.T) {
	tree, err := metrics.GetProcessTree(os.Getpid())
	if err != nil {
		t.Fatalf("GetProcessTree: %v", err)
	}
	for _, e := range tree {
		if e.Depth < 0 {
			t.Errorf("entry PID %d has negative depth %d", e.PID, e.Depth)
		}
	}
}

func TestProcTreeCollectorSafe(t *testing.T) {
	collector, err := metrics.NewProcTreeCollector(os.Getpid())
	if err != nil {
		t.Fatalf("failed to create collector: %v", err)
	}

	// 1. Should not panic
	// 2. Metrics should not be empty
	metrics, err := collector.Collect()
	if err != nil {
		t.Fatalf("failed to collect: %v", err)
	}

	if metrics.MemoryBytes == 0 {
		t.Errorf("MemoryBytes is 0")
	}

	// Run another time to hit the cache
	metrics2, err := collector.Collect()
	if err != nil {
		t.Fatalf("failed to collect second time: %v", err)
	}

	if metrics2.MemoryBytes == 0 {
		t.Errorf("MemoryBytes is 0 on cache hit")
	}
}
