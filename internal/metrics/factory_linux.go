//go:build linux

package metrics

// NewCollector creates the best available metrics collector for the given PID.
// It prefers Cgroup V2, but falls back to Process Tree aggregation.
func NewCollector(pid int) (Collector, error) {
	// 1. Try Cgroup V2
	if c, err := NewCgroupCollector(pid); err == nil {
		return c, nil
	}

	// 2. Fallback to Process Tree
	return NewProcTreeCollector(pid)
}
