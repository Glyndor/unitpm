package show

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/types"
)

func TestColorState(t *testing.T) {
	cases := []struct {
		in   types.ProcessState
		want string
	}{
		{types.StateRunning, "running"},
		{types.StateOnline, "online"},
		{types.StateStopped, "stopped"},
		{types.StateFailed, "failed"},
		{types.StateRestarting, "restarting"},
		{"", "-"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		got := colorState(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("colorState(%q)=%q, want substring %q", c.in, got, c.want)
		}
	}
}

func TestPidStr(t *testing.T) {
	if got := pidStr(0); !strings.Contains(got, "-") {
		t.Errorf("pidStr(0)=%q, want '-'", got)
	}
	if got := pidStr(42); got != "42" {
		t.Errorf("pidStr(42)=%q, want '42'", got)
	}
}
