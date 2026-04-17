package manager

import (
	"fmt"
	"log"
	"os"

	"github.com/Jaro-c/Lynx/internal/env"
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
		maxBytes: env.Int64("LYNX_LOG_MAX_BYTES", defaultRotateMaxBytes),
		keep:     env.Int("LYNX_LOG_KEEP", defaultRotateKeep),
	}
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
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		log.Printf("log-rotate: remove %s: %v", oldest, err)
	}

	// Shift: foo.log.(N-1) -> foo.log.N, ..., foo.log.1 -> foo.log.2
	for i := cfg.keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			log.Printf("log-rotate: rename %s → %s: %v", src, dst, err)
		}
	}

	// Current -> foo.log.1
	if err := os.Rename(path, path+".1"); err != nil {
		log.Printf("log-rotate: rename %s → %s.1: %v", path, path, err)
	}
}
