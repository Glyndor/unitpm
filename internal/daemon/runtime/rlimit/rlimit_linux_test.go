//go:build linux

package rlimit

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestApply_Zero_NoChange(t *testing.T) {
	// All-zero Limits should be a no-op; current process's rlimits
	// must be unchanged.
	var before unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &before); err != nil {
		t.Fatal(err)
	}
	if err := Apply(Limits{}); err != nil {
		t.Fatalf("Apply with zero Limits returned %v", err)
	}
	var after unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("Apply with zeros changed RLIMIT_NOFILE: %v -> %v", before, after)
	}
}

func TestApply_MaxFiles_LowersSoft(t *testing.T) {
	// Pick a value strictly below current hard limit so the test never
	// fails because of system caps.
	var cur unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &cur); err != nil {
		t.Fatal(err)
	}
	want := cur.Cur / 2
	if want < 16 {
		want = 16
	}
	if err := Apply(Limits{MaxFiles: want}); err != nil {
		t.Fatalf("Apply MaxFiles=%d: %v", want, err)
	}
	var got unix.Rlimit
	_ = unix.Getrlimit(unix.RLIMIT_NOFILE, &got)
	if got.Cur != want {
		t.Errorf("RLIMIT_NOFILE soft: got %d want %d", got.Cur, want)
	}
	// Restore (best effort — only if we still have headroom).
	_ = unix.Setrlimit(unix.RLIMIT_NOFILE, &cur)
}
