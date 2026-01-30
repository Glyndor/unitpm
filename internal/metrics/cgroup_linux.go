//go:build linux

package metrics

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CgroupCollector collects metrics using Cgroup V2.
type CgroupCollector struct {
	pid            int
	cgroupPath     string
	lastCPUTime    int64     // in microseconds
	lastSampleTime time.Time // time of last collection
}

// NewCgroupCollector creates a new collector for the given PID.
// It verifies that Cgroup V2 is available and the process has a valid cgroup.
func NewCgroupCollector(pid int) (*CgroupCollector, error) {
	// 1. Check if Cgroup V2 is mounted
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); os.IsNotExist(err) {
		return nil, errors.New("cgroup v2 not available")
	}

	// 2. Find cgroup path for PID
	cgroupPath, err := getCgroupPath(pid)
	if err != nil {
		return nil, err
	}

	// 3. Verify access to controller files
	// We need memory.current and cpu.stat
	if _, err := os.Stat(filepath.Join(cgroupPath, "memory.current")); err != nil {
		return nil, fmt.Errorf("memory controller not enabled for cgroup: %w", err)
	}
	if _, err := os.Stat(filepath.Join(cgroupPath, "cpu.stat")); err != nil {
		return nil, fmt.Errorf("cpu controller not enabled for cgroup: %w", err)
	}

	return &CgroupCollector{
		pid:        pid,
		cgroupPath: cgroupPath,
	}, nil
}

func (c *CgroupCollector) Collect() (Metrics, error) {
	now := time.Now()
	m := Metrics{
		Timestamp: now,
	}

	// 1. Read Memory
	memData, err := os.ReadFile(filepath.Join(c.cgroupPath, "memory.current"))
	if err != nil {
		return m, err
	}
	memBytes, err := strconv.ParseInt(strings.TrimSpace(string(memData)), 10, 64)
	if err != nil {
		return m, fmt.Errorf("invalid memory data: %w", err)
	}
	m.MemoryBytes = memBytes

	// 2. Read CPU
	cpuStatPath := filepath.Join(c.cgroupPath, "cpu.stat")
	cpuUsage, err := readCPUUsage(cpuStatPath)
	if err != nil {
		return m, err
	}

	// Calculate %
	if !c.lastSampleTime.IsZero() {
		deltaUsage := cpuUsage - c.lastCPUTime // microseconds
		deltaTime := now.Sub(c.lastSampleTime).Microseconds()

		if deltaTime > 0 {
			// Usage is total across all cores, so it can exceed 100% * 1 core.
			// This gives 1.0 for 100% of one core.
			// We want percentage, so * 100.
			m.CPUPercent = (float64(deltaUsage) / float64(deltaTime)) * 100.0
		}
	}

	// Update state
	c.lastCPUTime = cpuUsage
	c.lastSampleTime = now

	return m, nil
}

func getCgroupPath(pid int) (string, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: 0::/user.slice/...
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[1] == "" { // "" means unified/v2
			// Path is relative to /sys/fs/cgroup
			return filepath.Join("/sys/fs/cgroup", parts[2]), nil
		}
	}
	return "", fmt.Errorf("cgroup v2 path not found for pid %d", pid)
}

func readCPUUsage(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "usage_usec ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return strconv.ParseInt(parts[1], 10, 64)
			}
		}
	}
	return 0, errors.New("usage_usec not found in cpu.stat")
}
