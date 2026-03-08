package transport

// Identity represents the authenticated identity of an IPC client.
type Identity struct {
	UID string // User ID
	GID string // Group ID
	PID int    // Process ID
}

type contextKey string

// ContextKeyIdentity is the context key for the client identity.
const ContextKeyIdentity contextKey = "identity"
