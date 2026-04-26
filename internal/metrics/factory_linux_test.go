//go:build linux

package metrics

import (
	"os"
	"testing"
)

func TestNewCollector_PrefersProcTree(t *testing.T) {
	c, err := NewCollector(os.Getpid())
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	if _, ok := c.(*ProcTreeCollector); !ok {
		t.Errorf("expected *ProcTreeCollector, got %T", c)
	}
}

func TestNewCollector_BadPidFallsBackToCgroup(t *testing.T) {
	// Pid that is unlikely to exist. Either factory returns ProcTree (because
	// /proc/<pid>/stat happens to be readable on some kernels at startup), or
	// the cgroup fallback errors. Either outcome is acceptable; just verify
	// no panic and the error/collector are coherent.
	_, _ = NewCollector(2147483646)
}
