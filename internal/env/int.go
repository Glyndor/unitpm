package env

import (
	"os"
	"strconv"
)

// Int reads key from os.Environ, parses as int, and returns it when the
// value is strictly positive. Returns fallback when unset, malformed, or
// non-positive. Intended for daemon tuning knobs like LYNX_LOG_KEEP.
func Int(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// Int64 is the 64-bit variant of Int. Used for size caps in bytes.
func Int64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
