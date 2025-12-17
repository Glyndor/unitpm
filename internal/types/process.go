// Package types contains shared type definitions.
package types //nolint:revive

// ProcessState represents the current state of a process.
type ProcessState string

const (
	// StateRunning indicates the process is running.
	StateRunning ProcessState = "running"
	// StateOnline indicates the process is online and healthy.
	StateOnline ProcessState = "online"
	// StateStopped indicates the process is stopped.
	StateStopped ProcessState = "stopped"
	// StateFailed indicates the process has failed.
	StateFailed ProcessState = "failed"
	// StateExited indicates the process has exited normally.
	StateExited ProcessState = "exited"
	// StateRestarting indicates the process is restarting.
	StateRestarting ProcessState = "restarting"
)

// ProcessInfo represents the status of a managed process
// It contains only raw data, no formatting.
type ProcessInfo struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	Version   string       `json:"version"`
	Mode      string       `json:"mode"`
	PID       int          `json:"pid"`
	Uptime    int64        `json:"uptime_ms"` // Milliseconds
	Restarts  int          `json:"restarts"`
	State     ProcessState `json:"state"`
	CPU       float64      `json:"cpu"`          // Percentage 0.0-100.0
	Memory    int64        `json:"memory_bytes"` // Bytes
	User      string       `json:"user"`
	Watch     bool         `json:"watch"`
}
