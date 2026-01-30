// Package errs defines CLI error types.
package errs

// UsageError represents an error caused by incorrect CLI usage (invalid flags, args).
// When this error is returned, the CLI should display the error message
// followed by the command's help text.
type UsageError struct {
	Message string
}

func (e *UsageError) Error() string {
	return e.Message
}

// NewUsageError creates a new UsageError.
func NewUsageError(msg string) error {
	return &UsageError{Message: msg}
}
