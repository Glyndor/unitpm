// Package handlers provides the request handlers for the daemon.
package handlers

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/daemon/policy"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/types"
)

// nameRegex accepts human-friendly labels: letters, digits, spaces, dots,
// underscores, hyphens, and a small set of shell-safe punctuation
// (:, #, @, !, ,, (, ), +, =, &). 128 chars max. The colon is permitted
// because ResolveID splits on the FIRST colon only — addressing a name
// that contains colons still works via the explicit `namespace:name`
// form (e.g. `lynx show prod:TEST: Release 1`).
// namespaceRegex stays strict — no colon/space/# so `ns:name` parsing
// is unambiguous.
var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ._:#@!,()+=&-]{0,127}$`)
var namespaceRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// StartProcess handles the process start request with full validation and policy enforcement.
func StartProcess(
	mgr *manager.Manager,
	spec protocol.AppSpec,
	identity *transport.Identity,
	daemonPrivileged bool,
) (types.ProcessInfo, error) {
	if err := validateSpec(spec); err != nil {
		return types.ProcessInfo{}, err
	}

	if err := policy.AuthorizeStart(spec, identity, daemonPrivileged); err != nil {
		return types.ProcessInfo{}, err
	}

	if spec.EnvFile != "" {
		resolved, err := validateEnvFile(spec.EnvFile, identity)
		if err != nil {
			return types.ProcessInfo{}, err
		}
		spec.EnvFile = resolved
	}

	// Validate Cwd
	if spec.Cwd != "" {
		if len(spec.Cwd) > 4096 {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: cwd too long")
		}
		cwd := filepath.Clean(spec.Cwd)
		if !filepath.IsAbs(cwd) {
			var err error
			cwd, err = filepath.Abs(cwd)
			if err != nil {
				return types.ProcessInfo{}, errors.New("ERR_BAD_REQUEST: invalid cwd")
			}
		}
		resolved, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			return types.ProcessInfo{}, errors.New("ERR_BAD_REQUEST: invalid cwd")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return types.ProcessInfo{}, errors.New("ERR_BAD_REQUEST: invalid cwd")
		}
		for _, restricted := range []string{"/etc", "/proc", "/sys", "/boot", "/dev", "/run"} {
			if resolved == restricted || strings.HasPrefix(resolved, restricted+string(os.PathSeparator)) {
				return types.ProcessInfo{}, errors.New(
					"ERR_BAD_REQUEST: cwd is a restricted system directory; use --cwd to set a different path",
				)
			}
		}
		// Verify the daemon's own user can cd/stat into the resolved cwd.
		// In system mode the daemon runs as `lynx`, so if the client is root
		// inside /root the chdir() would later fail with a cryptic
		// `fork/exec ... permission denied`. Surface a clean error now.
		if f, err := os.Open(resolved); err != nil {
			return types.ProcessInfo{}, errors.New(
				"ERR_BAD_REQUEST: cwd is not accessible to the daemon user; " +
					"pass --cwd to a directory readable by the daemon " +
					"(e.g. /var/lib/lynx-pm or /tmp)",
			)
		} else {
			_ = f.Close()
		}
		spec.Cwd = resolved
	}

	// Start process via Manager
	return mgr.StartWithSpec(spec)
}

// validateEnvFile rejects env files the caller does not own. Prevents a
// lynxadm user from using the daemon to read another tenant's env staging
// files (owned by the daemon UID) or root-only secrets.
func validateEnvFile(path string, identity *transport.Identity) (string, error) {
	if len(path) > 4096 {
		return "", errors.New("ERR_LIMITS: env_file path too long")
	}
	clean := filepath.Clean(path)
	if strings.Contains(clean, ".."+string(os.PathSeparator)) ||
		strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("ERR_BAD_REQUEST: env_file must not contain '..'")
	}
	if !filepath.IsAbs(clean) {
		return clean, nil
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", errors.New("ERR_BAD_REQUEST: env_file not accessible")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", errors.New("ERR_BAD_REQUEST: env_file not accessible")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("ERR_BAD_REQUEST: env_file must be a regular file")
	}
	if identity == nil {
		return resolved, nil
	}
	callerUID, err := strconv.ParseUint(identity.UID, 10, 32)
	if err != nil {
		return "", errors.New("ERR_BAD_REQUEST: env_file: caller identity invalid")
	}
	if callerUID == 0 {
		return resolved, nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("ERR_INTERNAL: cannot stat env_file")
	}
	if uint64(stat.Uid) != callerUID {
		return "", errors.New("ERR_BAD_REQUEST: env_file not owned by caller")
	}
	return resolved, nil
}

func validateSpec(spec protocol.AppSpec) error {
	if spec.Exec.Type == "" {
		return errors.New("ERR_BAD_REQUEST: exec type is required")
	}

	switch spec.Exec.Type {
	case "command":
		if spec.Exec.Command == "" {
			return errors.New("ERR_BAD_REQUEST: command is required")
		}
		if len(spec.Exec.Command) > 4096 {
			return errors.New("ERR_LIMITS: command too long")
		}
	case "entry":
		if spec.Exec.Entry == "" {
			return errors.New("ERR_BAD_REQUEST: entry file is required")
		}
	default:
		return errors.New("ERR_BAD_REQUEST: invalid exec type")
	}

	if len(spec.Exec.Args) > 256 {
		return errors.New("ERR_LIMITS: too many arguments")
	}
	for _, arg := range spec.Exec.Args {
		if len(arg) > 4096 {
			return errors.New("ERR_LIMITS: argument too long")
		}
	}

	if spec.Name != "" {
		if !nameRegex.MatchString(spec.Name) {
			return errors.New("ERR_BAD_REQUEST: invalid name format")
		}
	}

	if spec.Namespace != "" {
		if !namespaceRegex.MatchString(spec.Namespace) {
			return errors.New("ERR_BAD_REQUEST: invalid namespace format")
		}
	}

	if spec.Logs != nil {
		if len(spec.Logs.Dir) > 4096 {
			return errors.New("ERR_LIMITS: log dir too long")
		}
		if spec.Logs.Mode != "" &&
			spec.Logs.Mode != "inherit" &&
			spec.Logs.Mode != "file" {
			return errors.New("ERR_BAD_REQUEST: invalid logs mode")
		}
		if spec.Logs.Format != "" &&
			spec.Logs.Format != "plain" &&
			spec.Logs.Format != "json" {
			return errors.New("ERR_BAD_REQUEST: invalid logs format")
		}
		if spec.Logs.Timestamp != "" &&
			spec.Logs.Timestamp != "none" &&
			spec.Logs.Timestamp != "rfc3339" &&
			spec.Logs.Timestamp != "unix" {
			return errors.New("ERR_BAD_REQUEST: invalid logs timestamp")
		}

		for _, p := range []string{spec.Logs.Dir, spec.Logs.Stdout, spec.Logs.Stderr} {
			if p == "" {
				continue
			}
			if len(p) > 4096 {
				return errors.New("ERR_LIMITS: log path too long")
			}
			clean := filepath.Clean(p)
			if strings.Contains(clean, ".."+string(os.PathSeparator)) ||
				strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
				return errors.New("ERR_BAD_REQUEST: log paths must not contain '..'")
			}
		}
		// Stdout/Stderr must be filenames under the app log dir, not absolute
		// paths. Logs.Dir may be absolute but is validated against an allowlist
		// of log roots downstream (GetLogDir).
		if filepath.IsAbs(filepath.Clean(spec.Logs.Stdout)) {
			return errors.New("ERR_BAD_REQUEST: logs.stdout must be a relative filename")
		}
		if filepath.IsAbs(filepath.Clean(spec.Logs.Stderr)) {
			return errors.New("ERR_BAD_REQUEST: logs.stderr must be a relative filename")
		}
	}

	if spec.Cron != "" {
		if len(spec.Cron) > 256 {
			return errors.New("ERR_LIMITS: cron spec too long")
		}
		if strings.Contains(spec.Cron, "\n") || strings.Contains(spec.Cron, "\r") {
			return errors.New("ERR_BAD_REQUEST: invalid cron spec")
		}
	}

	if len(spec.Env) > 128 {
		return errors.New("ERR_LIMITS: too many environment variables")
	}
	for k, v := range spec.Env {
		if len(k) > 256 {
			return errors.New("ERR_LIMITS: env key too long")
		}
		if len(v) > 8192 {
			return errors.New("ERR_LIMITS: env value too long")
		}
	}

	if err := validateStop(spec.Stop); err != nil {
		return err
	}
	if err := validateResources(spec.Resources); err != nil {
		return err
	}

	return nil
}

func validateStop(s *protocol.AppStop) error {
	if s == nil {
		return nil
	}
	if s.Signal != "" {
		if _, ok := manager.StopSignalByName[s.Signal]; !ok {
			return errors.New(
				"ERR_BAD_REQUEST: invalid stop signal; " +
					"allowed: SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2")
		}
	}
	if s.TimeoutMs != 0 && (s.TimeoutMs < 1000 || s.TimeoutMs > 300000) {
		return errors.New("ERR_LIMITS: stop.timeout_ms must be between 1000 and 300000 (1s to 5min)")
	}
	return nil
}

func validateResources(r *protocol.AppResources) error {
	if r == nil {
		return nil
	}
	if r.MemoryMaxBytes < 0 {
		return errors.New("ERR_BAD_REQUEST: resources.memory_max_bytes must be >= 0")
	}
	// 1 MiB floor when set — anything smaller is almost certainly a mistake
	// and many runtimes cannot even load.
	if r.MemoryMaxBytes != 0 && r.MemoryMaxBytes < 1024*1024 {
		return errors.New("ERR_LIMITS: resources.memory_max_bytes must be >= 1 MiB when set")
	}
	if r.CPUMaxPercent < 0 || r.CPUMaxPercent > 10000 {
		return errors.New("ERR_LIMITS: resources.cpu_max_percent must be between 0 and 10000")
	}
	if r.TasksMax < 0 {
		return errors.New("ERR_BAD_REQUEST: resources.tasks_max must be >= 0")
	}
	return nil
}
