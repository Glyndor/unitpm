//go:build linux

package runtime

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/runtime/landlock"
	"github.com/Jaro-c/Lynx/internal/daemon/runtime/rlimit"
)

func TestWrapSandbox_EmptyLynxBin(t *testing.T) {
	cmd := exec.Command("/bin/true")
	_, err := WrapSandbox(context.Background(), cmd, SandboxOptions{})
	if err == nil {
		t.Fatal("expected error for empty LynxBin, got nil")
	}
	if !strings.Contains(err.Error(), "LynxBin not set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapSandbox_WrapperPath(t *testing.T) {
	cmd := exec.Command("/bin/echo", "hello")
	opts := SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
		Cwd:     "/tmp",
	}

	wrapped, err := WrapSandbox(context.Background(), cmd, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wrapped.Path != "/usr/bin/lynxpm" {
		t.Errorf("wrapped.Path = %q, want /usr/bin/lynxpm", wrapped.Path)
	}
	if len(wrapped.Args) < 2 || wrapped.Args[1] != "_exec-sandbox" {
		t.Errorf("wrapped.Args = %v, want second arg to be _exec-sandbox", wrapped.Args)
	}
}

func TestWrapSandbox_ConfigEnvVarSet(t *testing.T) {
	cmd := exec.Command("/bin/true")
	cmd.Env = []string{"EXISTING=var"}
	opts := SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
		Cwd:     "/tmp",
		LogDir:  "/var/log/lynx",
		Limits:  rlimit.Limits{},
	}

	wrapped, err := WrapSandbox(context.Background(), cmd, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var configEnv string
	for _, e := range wrapped.Env {
		if strings.HasPrefix(e, "LYNX_SANDBOX_CONFIG=") {
			configEnv = e
			break
		}
	}
	if configEnv == "" {
		t.Fatal("LYNX_SANDBOX_CONFIG not found in wrapped env")
	}

	payload := strings.TrimPrefix(configEnv, "LYNX_SANDBOX_CONFIG=")
	if !strings.Contains(payload, `"cwd":"/tmp"`) {
		t.Errorf("config payload missing cwd: %s", payload)
	}
	if !strings.Contains(payload, `"command":"/bin/true"`) {
		t.Errorf("config payload missing command: %s", payload)
	}

	found := false
	for _, e := range wrapped.Env {
		if e == "EXISTING=var" {
			found = true
			break
		}
	}
	if !found {
		t.Error("original env not preserved in wrapped cmd")
	}
}

func TestWrapSandbox_IOPropagated(t *testing.T) {
	var buf bytes.Buffer
	cmd := exec.Command("/bin/true")
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	wrapped, err := WrapSandbox(context.Background(), cmd, SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wrapped.Stdout != &buf {
		t.Error("Stdout not propagated to wrapped cmd")
	}
	if wrapped.Stderr != os.Stderr {
		t.Error("Stderr not propagated to wrapped cmd")
	}
	if wrapped.Stdin != os.Stdin {
		t.Error("Stdin not propagated to wrapped cmd")
	}
}

func TestWrapSandbox_NamespaceFlags(t *testing.T) {
	cmd := exec.Command("/bin/true")
	wrapped, err := WrapSandbox(context.Background(), cmd, SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attr := wrapped.SysProcAttr
	if attr == nil {
		t.Fatal("SysProcAttr is nil")
	}

	want := uintptr(syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS)
	if attr.Cloneflags != want {
		t.Errorf("Cloneflags = %#x, want %#x", attr.Cloneflags, want)
	}
	if attr.GidMappingsEnableSetgroups {
		t.Error("GidMappingsEnableSetgroups must be false to prevent privilege escalation")
	}
	if !attr.Setpgid {
		t.Error("Setpgid should be true for process group isolation")
	}
}

func TestWrapSandbox_UIDMappedToCurrent(t *testing.T) {
	cmd := exec.Command("/bin/true")
	wrapped, err := WrapSandbox(context.Background(), cmd, SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uid := os.Getuid()
	gid := os.Getgid()
	attr := wrapped.SysProcAttr

	if len(attr.UidMappings) != 1 {
		t.Fatalf("UidMappings len = %d, want 1", len(attr.UidMappings))
	}
	if attr.UidMappings[0].ContainerID != 0 {
		t.Errorf("UidMappings ContainerID = %d, want 0", attr.UidMappings[0].ContainerID)
	}
	if attr.UidMappings[0].HostID != uid {
		t.Errorf("UidMappings HostID = %d, want %d", attr.UidMappings[0].HostID, uid)
	}
	if attr.UidMappings[0].Size != 1 {
		t.Errorf("UidMappings Size = %d, want 1", attr.UidMappings[0].Size)
	}

	if len(attr.GidMappings) != 1 {
		t.Fatalf("GidMappings len = %d, want 1", len(attr.GidMappings))
	}
	if attr.GidMappings[0].ContainerID != 0 {
		t.Errorf("GidMappings ContainerID = %d, want 0", attr.GidMappings[0].ContainerID)
	}
	if attr.GidMappings[0].HostID != gid {
		t.Errorf("GidMappings HostID = %d, want %d", attr.GidMappings[0].HostID, gid)
	}
}

func TestWrapSandbox_AllowListEncoded(t *testing.T) {
	cmd := exec.Command("/bin/true")
	allow := []landlock.PathAccess{
		{Path: "/srv/app", Read: true, Execute: true},
	}
	opts := SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
		Allow:   allow,
	}
	wrapped, err := WrapSandbox(context.Background(), cmd, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range wrapped.Env {
		if strings.HasPrefix(e, "LYNX_SANDBOX_CONFIG=") {
			if strings.Contains(e, "/srv/app") {
				return
			}
			t.Errorf("allow path /srv/app not in config: %s", e)
			return
		}
	}
	t.Error("LYNX_SANDBOX_CONFIG not found in env")
}

func TestWrapSandbox_NoErrorRegardlessOfLanglockSupport(t *testing.T) {
	// WrapSandbox must succeed even when Landlock is unsupported — it only
	// prints a warning. This verifies we never return an error for that path.
	cmd := exec.Command("/bin/true")
	_, err := WrapSandbox(context.Background(), cmd, SandboxOptions{
		LynxBin: "/usr/bin/lynxpm",
	})
	if err != nil {
		t.Fatalf("WrapSandbox should not error regardless of Landlock support: %v", err)
	}
}
