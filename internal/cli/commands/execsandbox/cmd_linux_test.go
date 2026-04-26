//go:build linux

package execsandbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("--help: %v", err)
	}
}

func TestRun_MissingConfigEnv(t *testing.T) {
	t.Setenv(envConfig, "")
	err := Run([]string{})
	if err == nil {
		t.Fatal("expected error when LYNX_SANDBOX_CONFIG is unset")
	}
	if !strings.Contains(err.Error(), "LYNX_SANDBOX_CONFIG") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_InvalidJSON(t *testing.T) {
	t.Setenv(envConfig, "not-json")
	err := Run([]string{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid sandbox config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_MissingCommand(t *testing.T) {
	b, _ := json.Marshal(Config{Cwd: "/tmp"})
	t.Setenv(envConfig, string(b))
	err := Run([]string{})
	if err == nil {
		t.Fatal("expected error when Command is empty")
	}
	if !strings.Contains(err.Error(), "missing command") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Note: we cannot test the full Run() happy path because it ends in
// syscall.Exec which replaces the current process.

func TestSerialize_Roundtrip(t *testing.T) {
	c := Config{
		Cwd:     "/tmp",
		Command: "/bin/echo",
		Args:    []string{"hello"},
	}
	s, err := Serialize(c)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Command != c.Command || len(got.Args) != 1 || got.Args[0] != "hello" {
		t.Errorf("roundtrip lost data: %+v", got)
	}
}

func TestConfigEnvVar(t *testing.T) {
	if ConfigEnvVar() != "LYNX_SANDBOX_CONFIG" {
		t.Errorf("unexpected env var name: %s", ConfigEnvVar())
	}
}

func TestWrapperCommand(t *testing.T) {
	parts := WrapperCommand("/usr/bin/lynx")
	if len(parts) != 2 || parts[0] != "/usr/bin/lynx" || parts[1] != "_exec-sandbox" {
		t.Errorf("unexpected wrapper cmd: %v", parts)
	}
}

func TestShellQuote(t *testing.T) {
	got := ShellQuote([]string{"a", "b", "c"})
	if got != "a b c" {
		t.Errorf("got %q", got)
	}
}

func TestGetSpec(t *testing.T) {
	s := GetSpec()
	if s.Name != "_exec-sandbox" {
		t.Errorf("name = %s", s.Name)
	}
	if !s.Hidden {
		t.Error("expected Hidden=true")
	}
}

func TestRun_RelativeCwd(t *testing.T) {
	b, _ := json.Marshal(Config{Cwd: "relative/path", Command: "echo"})
	t.Setenv(envConfig, string(b))
	err := Run(nil)
	if err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("got %v", err)
	}
}

func TestRun_PrctlOrMountFailsUnprivileged(t *testing.T) {
	// As an unprivileged caller, Run should fail at the prctl/mount stage
	// (unable to manipulate namespaces) — but never panic. Accept any error
	// after the JSON parse/Cwd checks pass.
	b, _ := json.Marshal(Config{Cwd: "/tmp", Command: "/bin/true"})
	t.Setenv(envConfig, string(b))
	err := Run(nil)
	if err == nil {
		// On the off chance we *are* in a sandbox, syscall.Exec replaced us
		// before we got here. That can't happen because the test process is
		// still running, so flag the unexpected nil.
		t.Fatal("expected error from sandbox setup outside a real namespace")
	}
}

func TestPrintHelpDoesNotPanic(t *testing.T) {
	PrintHelp()
}
