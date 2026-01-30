//go:build linux

package metrics

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultClkTck is the default clock ticks per second.
	// Most Linux systems use 100.
	defaultClkTck = 100.0
	// pageSize is usually 4KB.
	pageSize = 4096
)

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
	// Default to full tree scan for accuracy.
	// TODO: Optimization: Implement snapshot caching (once per tick) to reduce /proc scan overhead.
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

// findDescendants finds all descendant PIDs including the root.
// This is expensive as it scans /proc.
func (c *ProcTreeCollector) findDescendants(root int) ([]int, error) {
	// Map PPID -> []PID
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

		ppid, err := c.getPpid(pid)
		if err != nil {
			continue
		}
		tree[ppid] = append(tree[ppid], pid)
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

func (c *ProcTreeCollector) getPpid(pid int) (int, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}

	// Format: pid (comm) state ppid ...
	// Comm can contain spaces and parenthesis. Find last ')'
	s := string(data)
	lastParen := strings.LastIndex(s, ")")
	if lastParen == -1 || lastParen+2 >= len(s) {
		return 0, errors.New("invalid stat format")
	}

	parts := strings.Fields(s[lastParen+2:])
	if len(parts) < 1 {
		return 0, errors.New("invalid stat format")
	}

	return strconv.Atoi(parts[0])
}

func (c *ProcTreeCollector) readProcStat(pid int) (int64, int64, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, 0, err
	}

	s := string(data)
	lastParen := strings.LastIndex(s, ")")
	if lastParen == -1 {
		return 0, 0, errors.New("invalid stat format")
	}

	// Fields after comm (starting at index 0 relative to sub-slice)
	// 0: state, 1: ppid, ...
	// We need:
	// utime: 11 (14th field total)
	// stime: 12 (15th field total)
	// rss: 21 (24th field total)

	parts := strings.Fields(s[lastParen+2:])
	if len(parts) < 22 {
		return 0, 0, errors.New("stat too short")
	}

	utime, err := strconv.ParseInt(parts[11], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	stime, err := strconv.ParseInt(parts[12], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	rss, err := strconv.ParseInt(parts[21], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return utime + stime, rss, nil
}
