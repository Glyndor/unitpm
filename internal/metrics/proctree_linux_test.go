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
