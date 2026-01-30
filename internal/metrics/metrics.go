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
