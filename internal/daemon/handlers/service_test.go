//go:build linux

package handlers_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func selfIdentity() *transport.Identity {
	return &transport.Identity{
		UID: strconv.Itoa(os.Getuid()),
		GID: strconv.Itoa(os.Getgid()),
		PID: os.Getpid(),
	}
}

func baseSpec() protocol.AppSpec {
	return protocol.AppSpec{
		ID:    "00000000-0000-0000-0000-000000000001",
		Exec:  protocol.AppExec{Type: "command", Command: "echo"},
		RunAs: &protocol.RunAsPolicy{Mode: "self"},
	}
}

func TestValidateSpec_ExecBranches(t *testing.T) {
	mgr := manager.NewManager()
	cases := []struct {
		name string
		mod  func(s *protocol.AppSpec)
		want string
	}{
		{"invalid exec type", func(s *protocol.AppSpec) { s.Exec.Type = "weird" }, "invalid exec type"},
		{
			"entry missing",
			func(s *protocol.AppSpec) { s.Exec = protocol.AppExec{Type: "entry"} },
			"entry file is required",
		},
		{
			"arg too long",
			func(s *protocol.AppSpec) { s.Exec.Args = []string{strings.Repeat("a", 4097)} },
			"argument too long",
		},
		{
			"env value too long",
			func(s *protocol.AppSpec) { s.Env = map[string]string{"k": strings.Repeat("v", 8193)} },
			"env value too long",
		},
		{
			"env key too long",
			func(s *protocol.AppSpec) { s.Env = map[string]string{strings.Repeat("k", 257): "v"} },
			"env key too long",
		},
		{"namespace bad", func(s *protocol.AppSpec) { s.Namespace = "bad ns" }, "invalid namespace format"},
		{"cron too long", func(s *protocol.AppSpec) { s.Cron = strings.Repeat("a", 257) }, "cron spec too long"},
		{"cron newline", func(s *protocol.AppSpec) { s.Cron = "* * *\n* *" }, "invalid cron spec"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := baseSpec()
			c.mod(&s)
			_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err=%v want substring %q", err, c.want)
			}
		})
	}
}

func TestValidateSpec_LogsBranches(t *testing.T) {
	mgr := manager.NewManager()
	cases := []struct {
		name string
		logs *protocol.AppLogs
		want string
	}{
		{"bad mode", &protocol.AppLogs{Mode: "weird"}, "invalid logs mode"},
		{"bad format", &protocol.AppLogs{Format: "yaml"}, "invalid logs format"},
		{"bad timestamp", &protocol.AppLogs{Timestamp: "iso"}, "invalid logs timestamp"},
		{"dir too long", &protocol.AppLogs{Dir: strings.Repeat("a", 4097)}, "log dir too long"},
		{"path traversal", &protocol.AppLogs{Dir: "../../etc"}, "must not contain '..'"},
		{"abs stdout", &protocol.AppLogs{Stdout: "/tmp/x.log"}, "logs.stdout must be a relative filename"},
		{"abs stderr", &protocol.AppLogs{Stderr: "/tmp/x.log"}, "logs.stderr must be a relative filename"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := baseSpec()
			s.Logs = c.logs
			_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err=%v want %q", err, c.want)
			}
		})
	}
}

func TestValidateSpec_StopBranches(t *testing.T) {
	mgr := manager.NewManager()
	cases := []struct {
		name string
		stop *protocol.AppStop
		want string
	}{
		{"invalid signal", &protocol.AppStop{Signal: "SIGFAKE"}, "invalid stop signal"},
		{"timeout too small", &protocol.AppStop{TimeoutMs: 500}, "stop.timeout_ms"},
		{"timeout too big", &protocol.AppStop{TimeoutMs: 999_999}, "stop.timeout_ms"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := baseSpec()
			s.Stop = c.stop
			_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err=%v want %q", err, c.want)
			}
		})
	}
}

func TestValidateSpec_ResourcesBranches(t *testing.T) {
	mgr := manager.NewManager()
	cases := []struct {
		name string
		res  *protocol.AppResources
		want string
	}{
		{"neg memory", &protocol.AppResources{MemoryMaxBytes: -1}, "memory_max_bytes must be >= 0"},
		{"tiny memory", &protocol.AppResources{MemoryMaxBytes: 1024}, "memory_max_bytes must be >= 1 MiB"},
		{"neg cpu", &protocol.AppResources{CPUMaxPercent: -1}, "cpu_max_percent"},
		{"big cpu", &protocol.AppResources{CPUMaxPercent: 100_000}, "cpu_max_percent"},
		{"neg tasks", &protocol.AppResources{TasksMax: -1}, "tasks_max"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := baseSpec()
			s.Resources = c.res
			_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err=%v want %q", err, c.want)
			}
		})
	}
}

func TestValidateEnvFile_ViaStart(t *testing.T) {
	mgr := manager.NewManager()
	tmp := t.TempDir()

	envPath := filepath.Join(tmp, "env")
	if err := os.WriteFile(envPath, []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Run("too long", func(t *testing.T) {
		s := baseSpec()
		s.EnvFile = strings.Repeat("/a", 2200)
		_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
		if err == nil || !strings.Contains(err.Error(), "env_file path too long") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("dot-dot", func(t *testing.T) {
		s := baseSpec()
		s.EnvFile = "../foo"
		_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
		if err == nil || !strings.Contains(err.Error(), "must not contain '..'") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("not regular", func(t *testing.T) {
		s := baseSpec()
		s.EnvFile = tmp
		_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("not accessible", func(t *testing.T) {
		s := baseSpec()
		s.EnvFile = filepath.Join(tmp, "missing")
		_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
		if err == nil || !strings.Contains(err.Error(), "not accessible") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("not owned by caller", func(t *testing.T) {
		stat, ok := mustStat(t, envPath).Sys().(*syscall.Stat_t)
		if !ok {
			t.Skip("no syscall.Stat_t")
		}
		// Pretend caller is a different non-root UID than the file owner.
		uid := stat.Uid + 1
		ident := &transport.Identity{
			UID: strconv.FormatUint(uint64(uid), 10),
			GID: strconv.Itoa(os.Getgid()),
			PID: os.Getpid(),
		}
		s := baseSpec()
		s.EnvFile = envPath
		_, err := handlers.StartProcess(mgr, s, ident, false)
		if err == nil || !strings.Contains(err.Error(), "not owned by caller") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("relative skips owner check", func(t *testing.T) {
		s := baseSpec()
		s.EnvFile = "rel/env"
		// Should not produce an env_file error (start may fail later for other reasons,
		// but not an env_file ownership error).
		_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
		if err != nil && strings.Contains(err.Error(), "env_file") {
			t.Errorf("relative env_file should be allowed, got %v", err)
		}
	})
}

func TestStartProcess_CwdRestricted(t *testing.T) {
	mgr := manager.NewManager()
	s := baseSpec()
	s.Cwd = "/etc"
	_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
	if err == nil || !strings.Contains(err.Error(), "restricted system directory") {
		t.Errorf("got %v", err)
	}
}

func TestStartProcess_CwdTooLong(t *testing.T) {
	mgr := manager.NewManager()
	s := baseSpec()
	s.Cwd = strings.Repeat("a", 4097)
	_, err := handlers.StartProcess(mgr, s, selfIdentity(), false)
	if err == nil || !strings.Contains(err.Error(), "cwd too long") {
		t.Errorf("got %v", err)
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return info
}
