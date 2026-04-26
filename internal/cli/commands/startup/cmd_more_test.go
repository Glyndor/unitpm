//go:build linux

package startup

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestRun_HelpFlag(t *testing.T) {
	out := captureStdout(t, func() {
		if err := Run(nil, []string{"--help"}); err != nil {
			t.Errorf("Run --help err: %v", err)
		}
	})
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "lynxpm startup") {
		t.Errorf("help missing key sections; got:\n%s", out)
	}
}

func TestGetSpec(t *testing.T) {
	spec := GetSpec()
	if spec.Name != "startup" {
		t.Errorf("Name=%q", spec.Name)
	}
	if !strings.Contains(spec.Description, "system daemon") {
		t.Errorf("Description=%q", spec.Description)
	}
	if len(spec.Options) == 0 {
		t.Error("expected options")
	}
}

func TestRealRunner_Success(t *testing.T) {
	r := &RealRunner{}
	stdout, _, code, err := r.Run("true")
	if err != nil || code != 0 {
		t.Errorf("true: err=%v code=%d", err, code)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestRealRunner_NonZeroExit(t *testing.T) {
	r := &RealRunner{}
	_, _, code, err := r.Run("false")
	if err == nil {
		t.Error("expected error from false")
	}
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRealRunner_NotFound(t *testing.T) {
	r := &RealRunner{}
	_, _, code, err := r.Run("/no/such/binary/lynx-test-xyz")
	if err == nil {
		t.Error("expected error")
	}
	if code != 1 {
		t.Errorf("expected fallback exit 1 for non-ExitError, got %d", code)
	}
}

func TestRunSystemStartup_IsActiveFails(t *testing.T) {
	mockSystemd(t)
	getEuid = func() int { return 0 }
	runner := &MockRunner{Responses: map[string]MockResult{
		"systemctl is-active": {Err: errors.New("boom"), Stderr: "ohno"},
	}}
	err := Run(runner, nil)
	if err == nil || !strings.Contains(err.Error(), "lynxd service check failed") {
		t.Errorf("got %v", err)
	}
}

func TestRunSystemStartup_EnableFails(t *testing.T) {
	mockSystemd(t)
	getEuid = func() int { return 0 }
	runner := &MockRunner{Responses: map[string]MockResult{
		"systemctl enable": {Err: errors.New("nope"), Stderr: "denied"},
	}}
	err := Run(runner, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to enable lynxd") {
		t.Errorf("got %v", err)
	}
}

func TestRunUserStartup_Happy(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current unavailable: %v", err)
	}
	if cur.HomeDir == "" {
		t.Skip("no home dir for current user")
	}
	mockSystemd(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lynxd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))

	// runUserStartup writes inside user.Current().HomeDir; back up any pre-existing
	// unit file and restore on cleanup so we don't clobber a real install.
	unitPath := filepath.Join(cur.HomeDir, ".config", "systemd", "user", "lynxd.service")
	var backup []byte
	if data, err := os.ReadFile(unitPath); err == nil {
		backup = data
	}
	t.Cleanup(func() {
		if backup != nil {
			_ = os.WriteFile(unitPath, backup, 0o644)
		} else {
			_ = os.Remove(unitPath)
		}
	})

	getEuid = func() int { return 1000 }
	runner := &MockRunner{}
	out := captureStdout(t, func() {
		if err := Run(runner, nil); err != nil {
			t.Errorf("Run err: %v", err)
		}
	})
	if !strings.Contains(out, "Created unit file") {
		t.Errorf("unit creation message missing; out:\n%s", out)
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(data), "ExecStart=") {
		t.Error("unit missing ExecStart")
	}
	// Verify expected systemctl/loginctl calls were made.
	wantPrefixes := []string{"loginctl enable-linger", "systemctl --user daemon-reload", "systemctl --user enable"}
	for _, want := range wantPrefixes {
		found := false
		for _, c := range runner.Calls {
			if strings.HasPrefix(c, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing call %q; calls=%v", want, runner.Calls)
		}
	}
}

func TestRunUserStartup_LingerWarnContinues(t *testing.T) {
	if _, err := user.Current(); err != nil {
		t.Skipf("user.Current unavailable: %v", err)
	}
	mockSystemd(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lynxd")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))
	guardUserUnit(t)
	getEuid = func() int { return 1000 }

	runner := &MockRunner{Responses: map[string]MockResult{
		"loginctl enable-linger": {Err: errors.New("denied"), Stderr: "no perms"},
	}}
	out := captureStdout(t, func() {
		if err := Run(runner, nil); err != nil {
			t.Errorf("err=%v", err)
		}
	})
	if !strings.Contains(out, "Warning") {
		t.Errorf("expected linger warning; out:\n%s", out)
	}
}

func TestRunUserStartup_DaemonReloadFails(t *testing.T) {
	if _, err := user.Current(); err != nil {
		t.Skipf("user.Current unavailable: %v", err)
	}
	mockSystemd(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lynxd")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))
	guardUserUnit(t)
	getEuid = func() int { return 1000 }

	runner := &MockRunner{Responses: map[string]MockResult{
		"systemctl --user daemon-reload": {Err: errors.New("x"), Stderr: "y"},
	}}
	err := Run(runner, nil)
	if err == nil || !strings.Contains(err.Error(), "reload user daemon") {
		t.Errorf("got %v", err)
	}
}

func TestRunUserStartup_EnableFails(t *testing.T) {
	if _, err := user.Current(); err != nil {
		t.Skipf("user.Current unavailable: %v", err)
	}
	mockSystemd(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lynxd")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))
	guardUserUnit(t)
	getEuid = func() int { return 1000 }

	runner := &MockRunner{Responses: map[string]MockResult{
		"systemctl --user enable": {Err: errors.New("x"), Stderr: "y"},
	}}
	err := Run(runner, nil)
	if err == nil || !strings.Contains(err.Error(), "enable user lynxd") {
		t.Errorf("got %v", err)
	}
}

func TestRunUserStartup_LynxdNotFound(t *testing.T) {
	if _, err := user.Current(); err != nil {
		t.Skipf("user.Current unavailable: %v", err)
	}
	mockSystemd(t)
	tmp := t.TempDir()
	t.Setenv("PATH", tmp) // empty PATH dir
	guardUserUnit(t)
	getEuid = func() int { return 1000 }

	// Also blank /usr/sbin and /usr/local/bin checks: use override stat that returns NotExist.
	prevStat := stat
	stat = func(name string) (os.FileInfo, error) {
		if name == "/run/systemd/system" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { stat = prevStat })

	// runPlatformStartup uses the package-level stat, but runUserStartup uses os.Stat directly.
	// Skip if /usr/sbin/lynxd or /usr/local/bin/lynxd actually exists on this host.
	if _, err := os.Stat("/usr/sbin/lynxd"); err == nil {
		t.Skip("/usr/sbin/lynxd present")
	}
	if _, err := os.Stat("/usr/local/bin/lynxd"); err == nil {
		t.Skip("/usr/local/bin/lynxd present")
	}

	runner := &MockRunner{}
	err := Run(runner, nil)
	if err == nil || !strings.Contains(err.Error(), "lynxd binary not found") {
		t.Errorf("got %v", err)
	}
}

// guardUserUnit backs up any existing $HOME/.config/systemd/user/lynxd.service
// and restores it on cleanup so tests do not clobber a real install.
func guardUserUnit(t *testing.T) {
	t.Helper()
	cur, err := user.Current()
	if err != nil || cur.HomeDir == "" {
		return
	}
	unitPath := filepath.Join(cur.HomeDir, ".config", "systemd", "user", "lynxd.service")
	var backup []byte
	existed := false
	if data, err := os.ReadFile(unitPath); err == nil {
		backup = data
		existed = true
	}
	t.Cleanup(func() {
		if existed {
			_ = os.WriteFile(unitPath, backup, 0o644)
		} else {
			_ = os.Remove(unitPath)
		}
	})
}

func mockSystemd(t *testing.T) {
	t.Helper()
	prevStat := stat
	prevLook := lookPath
	prevEuid := getEuid
	stat = func(name string) (os.FileInfo, error) {
		if name == "/run/systemd/system" {
			return nil, nil
		}
		return prevStat(name)
	}
	lookPath = func(file string) (string, error) {
		if file == "systemctl" {
			return "/usr/bin/systemctl", nil
		}
		return prevLook(file)
	}
	t.Cleanup(func() { stat = prevStat; lookPath = prevLook; getEuid = prevEuid })
}
