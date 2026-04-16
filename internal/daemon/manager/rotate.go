package manager

import (
	"fmt"
	"os"
	"strconv"
)

// Log-rotation defaults. Overridable per-call (see rotateIfLarge) or globally
// via LYNX_LOG_MAX_BYTES / LYNX_LOG_KEEP env vars, resolved by currentRotateConfig.
const (
	defaultRotateMaxBytes int64 = 50 * 1024 * 1024 // 50 MiB
	defaultRotateKeep           = 3
)

type rotateConfig struct {
	maxBytes int64
	keep     int
}

func currentRotateConfig() rotateConfig {
	return rotateConfig{
		maxBytes: envInt64("LYNX_LOG_MAX_BYTES", defaultRotateMaxBytes),
		keep:     envInt("LYNX_LOG_KEEP", defaultRotateKeep),
	}
}

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
	rotateIfLargeCfg(path, currentRotateConfig())
}

func rotateIfLargeCfg(path string, cfg rotateConfig) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < cfg.maxBytes {
		return
	}

	// Delete the oldest backup if it exists.
	oldest := fmt.Sprintf("%s.%d", path, cfg.keep)
	_ = os.Remove(oldest)

	// Shift: foo.log.(N-1) -> foo.log.N, ..., foo.log.1 -> foo.log.2
	for i := cfg.keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		_ = os.Rename(src, dst)
	}

	// Current -> foo.log.1
	_ = os.Rename(path, path+".1")
}
