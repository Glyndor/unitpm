// Package protocol defines the IPC protocol structures and constants.
package protocol

import "fmt"

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

// MismatchData contains details about a protocol version mismatch.
type MismatchData struct {
	Supported int `json:"supported"`
	Received  int `json:"received"`
}

// Error returns the string representation of the RemoteError.
func (e *RemoteError) Error() string {
	return fmt.Sprintf("ipc error: [%s] %s", e.Code, e.Message)
}
