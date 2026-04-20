//go:build linux

package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	// defaultClkTck is the default clock ticks per second.
	// Most Linux systems use 100.
	defaultClkTck = 100.0
	// pageSize is usually 4KB.
	pageSize = 4096
)

// Global process tree cache to minimize /proc filesystem scanning overhead.
var (
	procCacheMu       sync.Mutex
	procTreeCache     map[int][]int
	procTreeCacheTime time.Time
)

// getGlobalTreeSnapshot retrieves a snapshot of the process parent-child relationships.
// It caches the tree for 1 second to avoid heavy I/O when multiple collectors run.
func getGlobalTreeSnapshot() (map[int][]int, error) {
	procCacheMu.Lock()
	defer procCacheMu.Unlock()

	now := time.Now()
	// 1-second cache TTL: when N collectors run at once (one per managed
	// process) only the first scan walks /proc; the rest share the snapshot.
	if now.Sub(procTreeCacheTime) < time.Second && procTreeCache != nil {
		return procTreeCache, nil
	}

	tree := make(map[int][]int)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		ppid, err := GetPPID(pid)
		if err != nil {
			continue
		}
		tree[ppid] = append(tree[ppid], pid)
	}

	procTreeCache = tree
	procTreeCacheTime = now
	return tree, nil
}

// GetPPID reads a single process's PPID directly from /proc/<pid>/stat.
// Handles the `comm` field's parens-and-spaces quirk via
// bytes.LastIndexByte(')') so process names with spaces still parse.
func GetPPID(pid int) (int, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}

	lastParen := bytes.LastIndexByte(data, ')')
	if lastParen == -1 || lastParen+2 >= len(data) {
		return 0, errors.New("invalid stat format")
	}

	parts := bytes.Fields(data[lastParen+2:])
	if len(parts) < 2 {
		return 0, errors.New("stat too short")
	}
	return strconv.Atoi(string(parts[1]))
}

// ProcTreeCollector collects metrics by aggregating process tree.
type ProcTreeCollector struct {
	rootPid        int
	lastTotalTicks int64
	lastSampleTime time.Time
}

// NewProcTreeCollector creates a new process tree collector.
func NewProcTreeCollector(pid int) (*ProcTreeCollector, error) {
	// Verify process exists
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return nil, err
	}
	return &ProcTreeCollector{rootPid: pid}, nil
}

// Collect collects metrics from the process tree.
func (c *ProcTreeCollector) Collect() (Metrics, error) {
	now := time.Now()
	m := Metrics{
		Timestamp: now,
	}

	// 1. Build Process Tree
	// Now cached and highly efficient
	pids, err := c.findDescendants(c.rootPid)
	if err != nil {
		// Fallback to just root PID if scan fails (e.g. permission error or race)
		pids = []int{c.rootPid}
	}

	var totalTicks int64
	var totalRSS int64

	// 2. Aggregate Metrics
	for _, pid := range pids {
		ticks, rss, err := c.readProcStat(pid)
		if err != nil {
			// Process might have died during scan, ignore
			continue
		}
		totalTicks += ticks
		totalRSS += rss
	}

	m.MemoryBytes = totalRSS * pageSize

	// 3. Calculate CPU %
	if !c.lastSampleTime.IsZero() {
		deltaTicks := totalTicks - c.lastTotalTicks
		deltaSec := now.Sub(c.lastSampleTime).Seconds()

		if deltaSec > 0 && deltaTicks >= 0 {
			// (ticks / HZ) / seconds * 100
			m.CPUPercent = (float64(deltaTicks) / defaultClkTck) / deltaSec * 100.0
		}
	}

	c.lastTotalTicks = totalTicks
	c.lastSampleTime = now

	return m, nil
}

// findDescendants finds all descendant PIDs including the root by querying the global snapshot cache.
func (c *ProcTreeCollector) findDescendants(root int) ([]int, error) {
	tree, err := getGlobalTreeSnapshot()
	if err != nil {
		return nil, err
	}

	// BFS traversal
	descendants := []int{root}
	queue := []int{root}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		children := tree[curr]
		descendants = append(descendants, children...)
		queue = append(queue, children...)
	}

	return descendants, nil
}

// readProcStat reads utime, stime, and rss without excessive string allocations.
func (c *ProcTreeCollector) readProcStat(pid int) (int64, int64, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, 0, err
	}

	lastParen := bytes.LastIndexByte(data, ')')
	if lastParen == -1 {
		return 0, 0, errors.New("invalid stat format")
	}

	// Fields after comm (starting at index 0 relative to sub-slice)
	// 0: state, 1: ppid, ...
	// We need:
	// utime: 11 (14th field total)
	// stime: 12 (15th field total)
	// rss: 21 (24th field total)
	parts := bytes.Fields(data[lastParen+2:])
	if len(parts) < 22 {
		return 0, 0, errors.New("stat too short")
	}

	utime, err := strconv.ParseInt(string(parts[11]), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	stime, err := strconv.ParseInt(string(parts[12]), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	rss, err := strconv.ParseInt(string(parts[21]), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return utime + stime, rss, nil
}
