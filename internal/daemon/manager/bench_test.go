//go:build linux

package manager

import (
	"os"
	"os/exec"
	"testing"
)

// BenchmarkWalkDescendants measures the cost of scanning /proc to collect
// the full descendant tree of a PID. This is called on every stop/kill
// operation and scales with the total number of processes on the system.
func BenchmarkWalkDescendants(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = walkDescendants(os.Getpid())
	}
}

// BenchmarkWalkDescendants_WithChildren benchmarks the same scan but with
// a realistic subtree: a shell script that spawns several children. This
// exercises the DFS over a non-trivial tree rather than a leaf PID.
func BenchmarkWalkDescendants_WithChildren(b *testing.B) {
	// Spawn a shell that keeps a few children alive for the duration.
	cmd := exec.Command("bash", "-c", "sleep 60 & sleep 60 & sleep 60 & wait")
	if err := cmd.Start(); err != nil {
		b.Skip("cannot start subprocess:", err)
	}
	b.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = walkDescendants(pid)
	}
}
