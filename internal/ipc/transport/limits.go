//go:build linux

package transport

import "time"

const (
	// MaxConnections is the maximum number of concurrent connections allowed.
	MaxConnections = 100
	// ReadTimeout is the timeout for reading from a connection.
	ReadTimeout = 5 * time.Second
	// WriteTimeout is the timeout for writing to a connection.
	WriteTimeout = 5 * time.Second
	// MaxMsgSize is the maximum size of a message.
	MaxMsgSize = 1024 * 1024 // 1MB
)
