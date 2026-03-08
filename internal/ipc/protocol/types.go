// Package protocol defines the IPC protocol types.
package protocol

import "github.com/Jaro-c/Lynx/internal/jsonx"

const (
	StatusError   = "error"
	StatusSuccess = "success"
)

// Request represents a standard IPC request.
type Request struct {
	Version int              `json:"version"`
	ID      string           `json:"id"`
	Command string           `json:"command"`
	Params  jsonx.RawMessage `json:"params,omitempty"`
}

// Response represents a standard IPC response.
type Response struct {
	Version int              `json:"version"`
	ID      string           `json:"id"`
	Status  string           `json:"status"` // "ok" | "error"
	Result  jsonx.RawMessage `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

// StartRequest represents the request payload for start command.
type StartRequest struct {
	ProtocolVersion int     `json:"protocol_version"`
	RequestID       string  `json:"request_id"`
	Type            string  `json:"type"` // "start"
	Spec            AppSpec `json:"spec"`
}

// StartResponse represents the response for a start request.
type StartResponse struct {
	ProtocolVersion int                `json:"protocol_version"`
	Type            string             `json:"type"`
	RequestID       string             `json:"request_id"`
	Ok              bool               `json:"ok"`
	Data            *StartResponseData `json:"data,omitempty"`
	Error           *StartError        `json:"error,omitempty"`
}

// StartResponseData contains success data for start.
type StartResponseData struct {
	ID        string `json:"id"`
	ProcID    string `json:"proc_id,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// StartError represents an error in start response.
type StartError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AppSpec contains the full specification of an application to be run.
type AppSpec struct {
	Version   int               `json:"version"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Exec      AppExec           `json:"exec"`
	Cwd       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	EnvFile   string            `json:"envFile,omitempty"`
	Logs      *AppLogs          `json:"logs,omitempty"`
	Restart   *AppRestart       `json:"restart,omitempty"`
	Cron      string            `json:"cron,omitempty"`
	RunAs     *RunAsPolicy      `json:"runAs,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
}

// AppExec defines execution details.
type AppExec struct {
	Type    string   `json:"type"` // "command" | "entry"
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Entry   string   `json:"entry,omitempty"`
	Runtime string   `json:"runtime,omitempty"`
	Shell   bool     `json:"shell,omitempty"`
}

// AppLogs defines logging configuration.
type AppLogs struct {
	Mode      string `json:"mode"` // "inherit" | "file"
	Dir       string `json:"dir,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Format    string `json:"format,omitempty"`    // "plain" | "json"
	Timestamp string `json:"timestamp,omitempty"` // "none" | "rfc3339" | "unix"
}

// AppRestart defines restart policy.
type AppRestart struct {
	Policy      string `json:"policy"` // "never" | "always" | "on-failure"
	MaxRetries  int    `json:"maxRetries,omitempty"`
	BackoffMs   int    `json:"backoffMs,omitempty"`
	BackoffType string `json:"backoffType,omitempty"` // "none" | "linear" | "expo"
	StopOnExit  []int  `json:"stopOnExit,omitempty"`
}

// RunAsPolicy defines isolation/user settings.
type RunAsPolicy struct {
	Mode string `json:"mode"` // "self" | "dynamic"
}
