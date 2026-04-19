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
	Stop      *AppStop          `json:"stop,omitempty"`
	Resources *AppResources     `json:"resources,omitempty"`
	Watch     *AppWatch         `json:"watch,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
}

// AppStop controls how the process is terminated. Zero values use sensible
// defaults (SIGTERM, 10s grace period).
type AppStop struct {
	// Signal is the signal name to deliver first. "SIGTERM" if empty.
	// Accepted: SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2.
	Signal string `json:"signal,omitempty"`
	// TimeoutMs is how long to wait for the process to exit after the
	// first signal before sending SIGKILL. Bounded to [1000, 300000].
	TimeoutMs int `json:"timeout_ms,omitempty"`
}

// ScaleResponse is the payload returned by the 'scale' IPC verb.
// Shared between manager.Scale and the CLI client to avoid struct drift.
type ScaleResponse struct {
	BaseName  string   `json:"base_name"`
	Namespace string   `json:"namespace"`
	Before    int      `json:"before"`
	After     int      `json:"after"`
	Created   []string `json:"created,omitempty"`
	Deleted   []string `json:"deleted,omitempty"`
}

// AppWatch configures filesystem watching. When enabled the daemon monitors
// the process cwd for changes and restarts the process automatically.
type AppWatch struct {
	Enabled bool     `json:"enabled"`
	Ignore  []string `json:"ignore,omitempty"`
}

// AppResources bounds the runtime resources a managed process may use.
// When the process runs under --isolation dynamic these map to systemd-run
// -p MemoryMax/CPUQuota/TasksMax. Under --isolation sandbox they map to
// setrlimit RLIMIT_AS/RLIMIT_NPROC (CPU% has no rlimit equivalent).
type AppResources struct {
	// MemoryMaxBytes is the hard memory ceiling. 0 means unlimited.
	MemoryMaxBytes int64 `json:"memory_max_bytes,omitempty"`
	// CPUMaxPercent caps CPU as a fraction of one core (0-100) or >100
	// for multi-core. 0 means unlimited.
	CPUMaxPercent int `json:"cpu_max_percent,omitempty"`
	// TasksMax is the maximum number of tasks (pthreads + subprocesses).
	// 0 means unlimited.
	TasksMax int `json:"tasks_max,omitempty"`
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
