package metrics

import (
	"time"
)

// Metrics holds the aggregated resource usage.
type Metrics struct {
	CPUPercent  float64   // CPU usage in percentage (0.0 - 100.0+)
	MemoryBytes int64     // Memory usage in bytes (RSS or cgroup usage)
	Timestamp   time.Time // Time when the metrics were collected
}

// Collector is the interface for gathering process metrics.
type Collector interface {
	// Collect gathers current metrics for the monitored process.
	Collect() (Metrics, error)
}
