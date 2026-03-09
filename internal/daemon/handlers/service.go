// Package handlers provides the request handlers for the daemon.
//
//nolint:gocognit,nestif,gocyclo,cyclop
package handlers

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/daemon/policy"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/types"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
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

	// Validate Cwd
	if spec.Cwd != "" {
		if len(spec.Cwd) > 4096 {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: cwd too long")
		}
		info, err := os.Stat(spec.Cwd)
		if err != nil || !info.IsDir() {
			return types.ProcessInfo{}, errors.New("ERR_BAD_REQUEST: invalid cwd")
		}
	}

	// Start process via Manager
	return mgr.StartWithSpec(spec)
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

	if spec.EnvFile != "" {
		if len(spec.EnvFile) > 4096 {
			return errors.New("ERR_LIMITS: env_file path too long")
		}
		if strings.Contains(spec.EnvFile, ".."+string(os.PathSeparator)) ||
			strings.Contains(spec.EnvFile, string(os.PathSeparator)+"..") {
			return errors.New("ERR_BAD_REQUEST: env_file must not contain '..'")
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
	return nil
}
