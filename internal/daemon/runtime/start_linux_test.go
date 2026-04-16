//go:build linux

package runtime

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestConfigureProcessIsolation_Self(t *testing.T) {
	cmd := exec.Command("/bin/true")
	if err := ConfigureProcessIsolation(cmd, protocol.RunAsPolicy{Mode: "self"}); err != nil {
		t.Errorf("self mode returned %v", err)
	}
	if cmd.SysProcAttr == nil {
		t.Error("expected SysProcAttr to be set even for self mode")
	}
}

func TestConfigureProcessIsolation_Empty(t *testing.T) {
	cmd := exec.Command("/bin/true")
	// Empty mode should be accepted as no-op (default branch).
	if err := ConfigureProcessIsolation(cmd, protocol.RunAsPolicy{Mode: ""}); err != nil {
		t.Errorf("empty mode returned %v", err)
	}
}

func TestConfigureProcessIsolation_ReservedModes(t *testing.T) {
	cmd := exec.Command("/bin/true")
	for _, mode := range []string{"app_user", "explicit_user"} {
		err := ConfigureProcessIsolation(cmd, protocol.RunAsPolicy{Mode: mode})
		if err == nil {
			t.Errorf("mode %q should be rejected", mode)
		}
		if !strings.Contains(err.Error(), "not implemented yet") {
			t.Errorf("mode %q: unexpected error %v", mode, err)
		}
	}
}
