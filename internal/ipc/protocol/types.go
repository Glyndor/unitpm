package protocol

import "github.com/Jaro-c/Lynx/internal/jsonx"

// StartRequest represents the request for the start command.
type StartRequest struct {
	ProtocolVersion int     `json:"protocol_version"`
	Type            string  `json:"type"` // must be "start"
	RequestID       string  `json:"request_id"`
	Spec            AppSpec `json:"spec"`
}

// AppSpec defines the persistent application specification (v1).
type AppSpec struct {
	Version   int               `json:"version"` // = 1
	Id        string            `json:"id"`      // UUID v4
	Name      string            `json:"name,omitempty"`
	CreatedAt string            `json:"createdAt"`
	Cwd       string            `json:"cwd"`
	Exec      AppExec           `json:"exec"`
	Env       map[string]string `json:"env,omitempty"`
	EnvFile   string            `json:"envFile,omitempty"`
	Logs      *AppLogs          `json:"logs,omitempty"`
	Restart   *AppRestart       `json:"restart,omitempty"`
	Cron      string            `json:"cron,omitempty"`
	RunAs     *RunAsPolicy      `json:"runAs,omitempty"` // Added for process isolation
}

type AppExec struct {
	Type    string   `json:"type"` // "command" or "entry"
	Command string   `json:"command,omitempty"`
	Entry   string   `json:"entry,omitempty"`
	Runtime string   `json:"runtime,omitempty"`
	Args    []string `json:"args,omitempty"`
	Shell   bool     `json:"shell,omitempty"` // Added for opt-in shell execution
}

type AppLogs struct {
	Mode   string `json:"mode"` // "inherit" | "pipe" | "file"
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type AppRestart struct {
	Policy     string `json:"policy"` // "never" | "always" | "on-failure"
	MaxRetries int    `json:"maxRetries,omitempty"`
	BackoffMs  int    `json:"backoffMs,omitempty"`
}

// StartSpec is deprecated, replaced by AppSpec.
// Keeping it if needed for backward compatibility or transition,
// but for this task we switch to AppSpec.

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
	Version   int              `json:"version"`
	ID        string           `json:"id"`
	Command   string           `json:"command"`
	Params    jsonx.RawMessage `json:"params,omitempty"`
	Timestamp int64            `json:"timestamp"`
}

// Response represents the standard IPC response envelope.
type Response struct {
	ID     string           `json:"id"`
	Status string           `json:"status"` // "success" or "error"
	Result jsonx.RawMessage `json:"result,omitempty"`
	Error  *Error           `json:"error,omitempty"`
}
