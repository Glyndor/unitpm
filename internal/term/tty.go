// Package term provides terminal styling and color output.
package term

import (
	"os"
)

// IsTTY returns true if stdout is a terminal.
func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ShouldUseColor decides if we should use colors based on TTY and env vars.
func ShouldUseColor() bool {
	// 1. Check if it's a TTY
	if !IsTTY() {
		return false
	}

	// 2. Check NO_COLOR (any value disables color)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 3. Check TERM
	term := os.Getenv("TERM")
	if term == "dumb" {
		return false
	}

	// On Unix, empty TERM usually means no capabilities.
	return term != ""
}
