package types

// ProcessState represents the current state of a process
type ProcessState string

const (
	StateRunning   ProcessState = "running"
	StateOnline    ProcessState = "online"
	StateStopped   ProcessState = "stopped"
	StateFailed    ProcessState = "failed"
	StateRestarting ProcessState = "restarting"
)

// ProcessInfo represents the status of a managed process
// It contains only raw data, no formatting.
type ProcessInfo struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Namespace   string       `json:"namespace"`
	Version     string       `json:"version"`
	Mode        string       `json:"mode"`
	PID         int          `json:"pid"`
	Uptime      int64        `json:"uptime_ms"`    // Milliseconds
	Restarts    int          `json:"restarts"`
	State       ProcessState `json:"state"`
	CPU         float64      `json:"cpu"`          // Percentage 0.0-100.0
	Memory      int64        `json:"memory_bytes"` // Bytes
	User        string       `json:"user"`
	Watch       bool         `json:"watch"`
}
