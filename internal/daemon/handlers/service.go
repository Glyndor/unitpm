package handlers

import (
	"fmt"
	"os"
	"regexp"

	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/daemon/policy"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/types"
)

// StartProcess handles the process start request with full validation and policy enforcement.
func StartProcess(mgr *manager.Manager, spec protocol.StartSpec, privileged bool) (types.ProcessInfo, error) {
	// Validation
	if spec.Cmd == "" {
		return types.ProcessInfo{}, fmt.Errorf("ERR_BAD_REQUEST: cmd is required")
	}
	if len(spec.Cmd) > 4096 {
		return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: cmd too long")
	}

	if len(spec.Args) > 256 {
		return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: too many arguments")
	}
	for _, arg := range spec.Args {
		if len(arg) > 4096 {
			return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: argument too long")
		}
	}

	if spec.Name != "" {
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`, spec.Name)
		if !matched {
			return types.ProcessInfo{}, fmt.Errorf("ERR_BAD_REQUEST: invalid name format")
		}
	}

	if len(spec.Env) > 128 {
		return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: too many environment variables")
	}
	for k, v := range spec.Env {
		if len(k) > 256 {
			return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: env key too long")
		}
		if len(v) > 8192 {
			return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: env value too long")
		}
	}

	if err := policy.AuthorizeStart(spec, privileged); err != nil {
		return types.ProcessInfo{}, err
	}

	// Validate Cwd
	if spec.Cwd != "" {
		if len(spec.Cwd) > 4096 {
			return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: cwd too long")
		}
		info, err := os.Stat(spec.Cwd)
		if err != nil || !info.IsDir() {
			return types.ProcessInfo{}, fmt.Errorf("ERR_BAD_REQUEST: invalid cwd")
		}
	}

	// Start process via Manager
	return mgr.StartWithSpec(spec)
}
