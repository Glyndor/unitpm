package types

// ProcessState represents the current state of a process
type ProcessState string

const (
	StateRunning ProcessState = "running"
	StateStopped ProcessState = "stopped"
	StateFailed  ProcessState = "failed"
)

// ProcessInfo represents the status of a managed process
type ProcessInfo struct {
	Name    string       `json:"name"`
	State   ProcessState `json:"state"`
	PID     int          `json:"pid,omitempty"`
	Uptime  string       `json:"uptime,omitempty"` // For now string, could be duration/seconds
	Memory  string       `json:"memory,omitempty"`
	CPU     string       `json:"cpu,omitempty"`
}
