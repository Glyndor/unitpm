package transport

// IPCClient defines the interface for communicating with the daemon.
type IPCClient interface {
	Call(command string, params any, result any) error
	Close() error
}
