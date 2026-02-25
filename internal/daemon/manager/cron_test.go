//go:build linux

package manager

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestNewProcess_CronScheduler(t *testing.T) { //nolint:testpackage
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
