// Package handlers provides the request handlers for the daemon.
package handlers

import (
	"errors"
	"os"
	"regexp"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/daemon/policy"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/types"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// StartProcess handles the process start request with full validation and policy enforcement.
func StartProcess(
	mgr *manager.Manager,
	spec protocol.StartSpec,
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

func validateSpec(spec protocol.StartSpec) error {
	if spec.Cmd == "" {
		return errors.New("ERR_BAD_REQUEST: cmd is required")
	}
	if len(spec.Cmd) > 4096 {
		return errors.New("ERR_LIMITS: cmd too long")
	}

	if len(spec.Args) > 256 {
		return errors.New("ERR_LIMITS: too many arguments")
	}
	for _, arg := range spec.Args {
		if len(arg) > 4096 {
			return errors.New("ERR_LIMITS: argument too long")
		}
	}

	if spec.Name != "" {
		if !nameRegex.MatchString(spec.Name) {
			return errors.New("ERR_BAD_REQUEST: invalid name format")
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
