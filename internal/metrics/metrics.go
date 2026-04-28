// Package metrics provides process resource usage metrics collection.
package metrics

import (
	"time"
)

// Metrics holds the resource usage statistics.
type Metrics struct {
	Timestamp   time.Time
	MemoryBytes int64
	CPUPercent  float64
}

// Collector defines the interface for collecting metrics.
type Collector interface {
	Collect() (Metrics, error)
}

// ChildStat holds per-PID resource stats for one process in a tree.
type ChildStat struct {
	PID         int    `json:"pid"`
	Comm        string `json:"comm"`         // process name from /proc/<pid>/comm
	Depth       int    `json:"depth"`        // 0 = root, 1 = direct child, etc.
	MemoryBytes int64  `json:"memory_bytes"` // RSS in bytes
}
