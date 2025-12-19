package protocol

import "encoding/json"

// StartRequest represents the request for the start command.
type StartRequest struct {
	ProtocolVersion int       `json:"protocol_version"`
	Type            string    `json:"type"` // must be "start"
	RequestID       string    `json:"request_id"`
	Spec            StartSpec `json:"spec"`
}

// StartSpec defines the process to start.
type StartSpec struct {
	Name  string            `json:"name,omitempty"`
	Cmd   string            `json:"cmd"`
	Args  []string          `json:"args"`
	Cwd   string            `json:"cwd,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
	Stdio string            `json:"stdio"` // "inherit" | "pipe" | "file"
	RunAs RunAsPolicy       `json:"run_as"`
}

// RunAsPolicy defines the user execution policy.
type RunAsPolicy struct {
	Mode     string `json:"mode"` // "self" | "app_user" | "explicit_user"
	Username string `json:"username,omitempty"`
}

// StartResponse represents the response for the start command.
type StartResponse struct {
	ProtocolVersion int                `json:"protocol_version"`
	Type            string             `json:"type"` // "start_result"
	RequestID       string             `json:"request_id"`
	Ok              bool               `json:"ok"`
	Data            *StartResponseData `json:"data,omitempty"`
	Error           *StartError        `json:"error,omitempty"`
}

// StartResponseData contains details of the started process.
type StartResponseData struct {
	ProcID    string `json:"proc_id"`
	PID       int    `json:"pid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// StartError represents a structured error in the StartResponse.
type StartError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Request represents the standard IPC request envelope.
type Request struct {
	Version   int             `json:"version"`
	ID        string          `json:"id"`
	Command   string          `json:"command"`
	Params    json.RawMessage `json:"params,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// Response represents the standard IPC response envelope.
type Response struct {
	ID     string          `json:"id"`
	Status string          `json:"status"` // "success" or "error"
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}
