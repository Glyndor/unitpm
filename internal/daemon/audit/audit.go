// Package audit writes a JSON-lines record of every destructive action the
// daemon takes (start, stop, delete, reload, restart, reset, flush). The log
// is intended for compliance and post-mortem forensics; it is ON in system
// mode (/var/log/lynx-pm/audit.log) and off in user mode where the daemon
// is already scoped to a single user.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is one line in the audit log.
type Event struct {
	Time    string `json:"time"`            // RFC3339 of event
	Action  string `json:"action"`          // start, stop, delete, reload, restart, reset, flush
	UID     string `json:"uid,omitempty"`   // caller's UID via SO_PEERCRED
	GID     string `json:"gid,omitempty"`   // caller's GID
	PID     int    `json:"pid,omitempty"`   // caller's PID
	Target  string `json:"target"`          // resolved process id
	Name    string `json:"name,omitempty"`  // process name
	NS      string `json:"ns,omitempty"`    // namespace
	Success bool   `json:"success"`         // outcome
	Error   string `json:"error,omitempty"` // message when !Success
}

// Logger is safe for concurrent use. The zero value is a disabled logger —
// Log() is a no-op. Use Open() to obtain an enabled one.
type Logger struct {
	mu sync.Mutex
	w  io.Writer
}

var disabled = &Logger{}

// Disabled returns a no-op logger — the fallback when audit is intentionally
// off (user mode) or when opening the log file failed at startup.
func Disabled() *Logger { return disabled }

// Open creates or opens the audit log at path, creating parent directories
// if needed. Permissions are 0600 (owner-only). Returns a Disabled logger
// on any filesystem error so the daemon never fails to start because of
// audit setup.
func Open(path string) *Logger {
	if path == "" {
		return disabled
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return disabled
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return disabled
	}
	return &Logger{w: f}
}

// Log writes one event. Best-effort: write errors are swallowed so a full
// disk cannot break the IPC path.
func (l *Logger) Log(e Event) {
	if l == nil || l.w == nil {
		return
	}
	e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.w, "%s\n", b)
}
