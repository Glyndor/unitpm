package show

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
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

func TestGitStr(t *testing.T) {
	if got := gitStr(types.ProcessInfo{}); !strings.Contains(got, "-") {
		t.Errorf("gitStr empty=%q", got)
	}
	got := gitStr(types.ProcessInfo{GitBranch: "main", GitCommit: "abc"})
	if !strings.Contains(got, "main") || !strings.Contains(got, "abc") {
		t.Errorf("gitStr=%q", got)
	}
	dirty := gitStr(types.ProcessInfo{GitBranch: "main", GitCommit: "abc", GitDirty: true})
	if !strings.Contains(dirty, "*") {
		t.Errorf("dirty marker missing: %q", dirty)
	}
}

func TestWatchStr(t *testing.T) {
	if !strings.Contains(watchStr(true), "enabled") {
		t.Error("true should produce 'enabled'")
	}
	if !strings.Contains(watchStr(false), "disabled") {
		t.Error("false should produce 'disabled'")
	}
}

func TestBoolDimmed(t *testing.T) {
	if !strings.Contains(boolDimmed(true), "true") {
		t.Error("true")
	}
	if !strings.Contains(boolDimmed(false), "false") {
		t.Error("false")
	}
}

func TestJoinArgs(t *testing.T) {
	if got := joinArgs(nil); got != "" {
		t.Errorf("nil args=%q", got)
	}
	if got := joinArgs([]string{"a", "b"}); got != "a b" {
		t.Errorf("simple=%q", got)
	}
	got := joinArgs([]string{"a b", "c"})
	if got != `"a b" c` {
		t.Errorf("quoted=%q", got)
	}
}

func TestJoinLogPath(t *testing.T) {
	cases := []struct {
		dir, file, want string
	}{
		{"", "", ""},
		{"/var/log", "", ""},
		{"", "stdout.log", "stdout.log"},
		{"/var/log", "/etc/abs.log", "/etc/abs.log"},
		{"/var/log", "stdout.log", "/var/log/stdout.log"},
	}
	for _, c := range cases {
		if got := joinLogPath(c.dir, c.file); got != c.want {
			t.Errorf("joinLogPath(%q,%q)=%q want %q", c.dir, c.file, got, c.want)
		}
	}
}

func TestIntOrHelpers(t *testing.T) {
	if !strings.Contains(intOrDash(0), "-") {
		t.Error("intOrDash(0)")
	}
	if intOrDash(5) != "5" {
		t.Error("intOrDash(5)")
	}
	if !strings.Contains(intOrUnlimited(0), "unlimited") {
		t.Error("intOrUnlimited(0)")
	}
	if intOrUnlimited(7) != "7" {
		t.Error("intOrUnlimited(7)")
	}
	if !strings.Contains(memOrUnlimited(0), "unlimited") {
		t.Error("memOrUnlimited(0)")
	}
	if got := memOrUnlimited(2 * 1024 * 1024); got == "" {
		t.Error("memOrUnlimited 2MiB empty")
	}
	if !strings.Contains(cpuOrUnlimited(0), "unlimited") {
		t.Error("cpuOrUnlimited(0)")
	}
	if !strings.Contains(cpuOrUnlimited(150), "150%") {
		t.Errorf("cpuOrUnlimited(150)=%q", cpuOrUnlimited(150))
	}
}

func TestStrDefaultNonEmpty(t *testing.T) {
	if strDefault("", "x") != "x" {
		t.Error("strDefault empty")
	}
	if strDefault("a", "x") != "a" {
		t.Error("strDefault preserves")
	}
	if nonEmpty("", "b") != "b" {
		t.Error("nonEmpty fallback")
	}
	if nonEmpty("a", "b") != "a" {
		t.Error("nonEmpty preserves")
	}
}

func TestMaskSecret(t *testing.T) {
	got := maskSecret("API_TOKEN", "abc")
	if got != strings.Repeat("*", 8) && !strings.Contains(got, "*") {
		t.Errorf("token not masked: %q", got)
	}
	if maskSecret("PORT", "") != "" {
		t.Error("empty value should stay empty")
	}
	if got := maskSecret("PORT", "8080"); got != "8080" {
		t.Errorf("non-secret leaked through transform: %q", got)
	}
	for _, k := range []string{"PASSWORD", "PASSWD", "MY_KEY", "CREDENTIALS", "PRIVATE_KEY"} {
		if !strings.Contains(maskSecret(k, "v"), "*") {
			t.Errorf("%s not masked", k)
		}
	}
}

func TestRenderRestartFull(t *testing.T) {
	// Just exercise the function end-to-end; output goes to stdout, we just
	// want coverage of the branches that fire when fields are set.
	spec := protocol.AppSpec{
		Restart: &protocol.AppRestart{
			Policy: "always", MaxRetries: 3, BackoffMs: 1000, BackoffType: "expo",
			StopOnExit: []int{0, 2},
		},
	}
	renderRestart(spec)
	renderRestart(protocol.AppSpec{}) // nil branch

	renderEnv(protocol.AppSpec{
		EnvFile: "/tmp/env",
		Env:     map[string]string{"FOO": "bar", "API_TOKEN": "xyz"},
	})
	renderEnv(protocol.AppSpec{}) // nil branch

	renderLogs(protocol.AppSpec{Logs: &protocol.AppLogs{Mode: "file", Dir: "/var/log", Stdout: "out.log"}})
	renderLogs(protocol.AppSpec{}) // nil branch

	renderResources(protocol.AppSpec{Resources: &protocol.AppResources{
		MemoryMaxBytes: 512 * 1024 * 1024, CPUMaxPercent: 200, TasksMax: 100,
	}})
	renderResources(protocol.AppSpec{Resources: &protocol.AppResources{}}) // all-zero shortcut
	renderResources(protocol.AppSpec{})                                    // nil

	renderStop(protocol.AppSpec{Stop: &protocol.AppStop{Signal: "SIGTERM", TimeoutMs: 1000}})
	renderStop(protocol.AppSpec{})

	renderIsolation(protocol.AppSpec{RunAs: &protocol.RunAsPolicy{Mode: "self"}})
	renderIsolation(protocol.AppSpec{})

	renderSchedule(protocol.AppSpec{Cron: "* * * * *"})
	renderSchedule(protocol.AppSpec{})

	renderWatch(protocol.AppSpec{Watch: &protocol.AppWatch{Enabled: true, Ignore: []string{"node_modules"}}})
	renderWatch(protocol.AppSpec{Watch: &protocol.AppWatch{}})
	renderWatch(protocol.AppSpec{})
}

func TestPrintHelp(t *testing.T) {
	PrintHelp()
}
