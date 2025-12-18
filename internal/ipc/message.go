package ipc

import (
	"encoding/json"
	"fmt"
)

const (
	// Version is the current protocol version.
	Version = 1
)

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

// Error represents a structured error in the response.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// RemoteError wraps an IPC error response.
type RemoteError struct {
	Code    string
	Message string
	Data    any
}

// ProtocolMismatchData contains details about a protocol version mismatch.
type ProtocolMismatchData struct {
	Supported int `json:"supported"`
	Received  int `json:"received"`
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("ipc error: [%s] %s", e.Code, e.Message)
}
