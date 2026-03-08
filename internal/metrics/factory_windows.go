//go:build windows

package metrics

import "errors"

// NewCollector creates the best available metrics collector for the given PID.
func NewCollector(pid int) (Collector, error) {
	return nil, errors.New("metrics collection not supported on Windows")
}
