//go:build linux

package metrics

// NewCollector creates the best available metrics collector for the given PID.
// ProcTree is preferred because it reports per-process memory accurately via /proc.
// Cgroup V2 reads the entire session cgroup (which may contain many processes),
// making memory metrics unreliable unless the process is in a dedicated cgroup
// (e.g. dynamic isolation mode with systemd-run DynamicUser).
func NewCollector(pid int) (Collector, error) {
	// 1. Prefer Process Tree (accurate per-process via /proc/{pid}/stat)
	if c, err := NewProcTreeCollector(pid); err == nil {
		return c, nil
	}

	// 2. Fallback to Cgroup V2 (only accurate if process is in a dedicated cgroup)
	return NewCgroupCollector(pid)
}
