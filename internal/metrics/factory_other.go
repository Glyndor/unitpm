//go:build !linux

package metrics

// DummyCollector is a no-op collector for unsupported platforms.
type DummyCollector struct{}

// Collect returns empty metrics.
func (d *DummyCollector) Collect() (Metrics, error) {
	return Metrics{}, nil
}

// NewCollector returns a dummy collector.
func NewCollector(pid int) (Collector, error) {
	return &DummyCollector{}, nil
}
