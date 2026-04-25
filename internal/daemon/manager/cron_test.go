//go:build linux

package manager

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/types"
)

func TestNewProcess_CronScheduler(t *testing.T) {
	// 1. Create a spec with a cron schedule
	spec := protocol.AppSpec{
		Name: "test-cron",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "echo",
			Args:    []string{"hello"},
		},
		Cron: "@every 60s",
		Restart: &protocol.AppRestart{
			Policy: "on-failure",
		},
	}

	// 2. Initialize process
	p, err := NewProcess("123e4567-e89b-12d3-a456-426614174000", spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}

	// 3. Verify scheduler is initialized
	if p.scheduler == nil {
		t.Error("Scheduler should be initialized when Cron spec is present")
	}

	// 4. Verify scheduler is NOT initialized if Cron is empty
	spec.Cron = ""
	p2, err := NewProcess("123e4567-e89b-12d3-a456-426614174001", spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}
	if p2.scheduler != nil {
		t.Error("Scheduler should NOT be initialized when Cron spec is empty")
	}
}

// TestCron_FiresAndIncrementsRestarts proves the cron callback wired in
// NewProcess actually invokes Restart() and bumps info.Restarts. Triggers
// the registered Job synchronously to avoid a real 5s+ wait.
func TestCron_FiresAndIncrementsRestarts(t *testing.T) {
	restore := setupTestEnv(t)
	defer restore()

	id := uuid.Must(uuid.NewV7()).String()
	spec := protocol.AppSpec{
		Version: 1,
		ID:      id,
		Name:    "cron-fire-test",
		Exec: protocol.AppExec{
			Type:    "command",
			Command: "sleep",
			Args:    []string{"30"},
		},
		Cron: "@every 5s",
	}

	p, err := NewProcess(id, spec)
	if err != nil {
		t.Fatalf("NewProcess failed: %v", err)
	}
	if p.scheduler == nil {
		t.Fatal("scheduler nil after NewProcess with Cron spec")
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop(true) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if p.Info().State == types.StateRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for running state, got %s", p.Info().State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	entries := p.scheduler.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 cron entry, got %d", len(entries))
	}

	for i := 1; i <= 2; i++ {
		entries[0].Job.Run()

		deadline := time.Now().Add(3 * time.Second)
		for {
			if p.Info().Restarts == i && p.Info().State == types.StateRunning {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("tick %d: want Restarts=%d state=Running, got Restarts=%d state=%s",
					i, i, p.Info().Restarts, p.Info().State)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
