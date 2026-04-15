package manager

import (
	"fmt"
	"os"
	"strconv"
)

// Log-rotation knobs. Both overridable via env so sysadmins can tune without
// recompiling. Defaults match a conservative "never blow up the disk" policy.
var (
	// rotateMaxBytes is the size threshold above which a log file gets
	// rotated on the next spawn of the owning process.
	rotateMaxBytes int64 = envInt64("LYNX_LOG_MAX_BYTES", 50*1024*1024) // 50 MiB
	// rotateKeep is how many rotated backups (foo.log.1 .. foo.log.N) we
	// keep. Older rotations are deleted.
	rotateKeep = envInt("LYNX_LOG_KEEP", 3)
)

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// rotateIfLarge checks the given log path and, if it exceeds the size
// threshold, shifts existing rotations (foo.log.N-1 -> foo.log.N) and
// renames the current file to foo.log.1. Never returns an error: rotation
// is best-effort and must not block a spawn.
func rotateIfLarge(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < rotateMaxBytes {
		return
	}

	// Delete the oldest backup if it exists.
	oldest := fmt.Sprintf("%s.%d", path, rotateKeep)
	_ = os.Remove(oldest)

	// Shift: foo.log.(N-1) -> foo.log.N, ..., foo.log.1 -> foo.log.2
	for i := rotateKeep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		_ = os.Rename(src, dst)
	}

	// Current -> foo.log.1
	_ = os.Rename(path, path+".1")
}
