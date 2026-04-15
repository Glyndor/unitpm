//go:build linux

// Package rlimit wraps setrlimit for the sandbox runtime. Called from the
// child wrapper before execve.
package rlimit

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Limits bundles the caps applied by the sandbox. Zero means "do not set".
type Limits struct {
	// MemoryBytes is the address-space cap (RLIMIT_AS). The process is killed
	// with SIGSEGV if it exceeds this.
	MemoryBytes uint64
	// CPUSeconds is the CPU-time cap (RLIMIT_CPU). Process receives SIGXCPU.
	CPUSeconds uint64
	// MaxProcs is the per-user process cap (RLIMIT_NPROC).
	MaxProcs uint64
	// MaxFiles is the per-process open-files cap (RLIMIT_NOFILE).
	MaxFiles uint64
}

// Apply sets each non-zero limit on the current process. Soft and hard are
// set to the same value so the child cannot raise them back.
func Apply(l Limits) error {
	if l.MemoryBytes > 0 {
		if err := setOne(unix.RLIMIT_AS, l.MemoryBytes); err != nil {
			return fmt.Errorf("RLIMIT_AS: %w", err)
		}
	}
	if l.CPUSeconds > 0 {
		if err := setOne(unix.RLIMIT_CPU, l.CPUSeconds); err != nil {
			return fmt.Errorf("RLIMIT_CPU: %w", err)
		}
	}
	if l.MaxProcs > 0 {
		if err := setOne(unix.RLIMIT_NPROC, l.MaxProcs); err != nil {
			return fmt.Errorf("RLIMIT_NPROC: %w", err)
		}
	}
	if l.MaxFiles > 0 {
		if err := setOne(unix.RLIMIT_NOFILE, l.MaxFiles); err != nil {
			return fmt.Errorf("RLIMIT_NOFILE: %w", err)
		}
	}
	return nil
}

func setOne(which int, value uint64) error {
	rl := unix.Rlimit{Cur: value, Max: value}
	return unix.Setrlimit(which, &rl)
}
