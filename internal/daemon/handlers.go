// Package daemon wires the lynxd command handlers into the IPC server and
// owns the daemon-side lifecycle.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Jaro-c/Lynx/internal/daemon/audit"
	"github.com/Jaro-c/Lynx/internal/daemon/handlers"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/paths"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/version"
)

// DataDir is the standard data directory on Linux (/var/lib/lynx-pm).
const DataDir = paths.DataDir

// RegisterHandlers registers all daemon IPC handlers. Pass audit.Disabled()
// to disable audit logging (user mode); pass an audit.Open(path) logger to
// emit a JSONL line per destructive action.
//
//nolint:funlen // dispatcher inlines 60+ handler registrations for locality
func RegisterHandlers(server *transport.Server, mgr *manager.Manager, privileged bool, auditor *audit.Logger) {
	// Register ping handler
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler (audited via wrapping)
	startH := handlers.StartHandler(mgr, privileged)
	server.Register("start", func(ctx context.Context, params jsonx.RawMessage) (jsonx.RawMessage, error) {
		res, err := startH(ctx, params)
		if err != nil {
			auditEvent(auditor, ctx, "start", "", "", "", false, err)
			return nil, err
		}
		var data protocol.StartResponseData
		_ = jsonx.Unmarshal(res, &data)
		id := data.ProcID
		if id == "" {
			id = data.ID
		}
		name, ns := processMeta(mgr, id)
		auditEvent(auditor, ctx, "start", id, name, ns, true, nil)
		return res, nil
	})

	// Register stop handler
	server.Register("stop", func(
		ctx context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			auditEvent(auditor, ctx, "stop", args.ID, "", "", false, err)
			return nil, err
		}

		name, ns := processMeta(mgr, id)
		// Check state before stopping to determine if the process was running
		wasRunning := false
		if proc, ok := mgr.Get(id); ok {
			info := proc.Info()
			wasRunning = info.State == "running" || info.State == "restarting" || info.State == "online"
		}

		if err := mgr.Stop(id); err != nil {
			auditEvent(auditor, ctx, "stop", id, name, ns, false, err)
			return nil, err
		}

		auditEvent(auditor, ctx, "stop", id, name, ns, true, nil)
		return jsonx.Marshal(map[string]any{"status": "stopped", "id": id, "was_running": wasRunning})
	})

	// Simple id-in / {status,id}-out handlers.
	registerIDHandler(server, mgr, auditor, "restart", "restarted", (*manager.Manager).Restart)

	// Register delete handler
	server.Register("delete", func(
		ctx context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID    string `json:"id"`
			Purge bool   `json:"purge"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			auditEvent(auditor, ctx, "delete", args.ID, "", "", false, err)
			return nil, err
		}

		// Snapshot name+ns BEFORE deletion so audit line has useful metadata.
		delName, delNS := processMeta(mgr, id)

		// Prepare for purge (resolve log dir)
		var appLogDir string
		if args.Purge {
			if proc, ok := mgr.Get(id); ok {
				// Re-implement log dir logic briefly
				s := proc.Spec()
				configuredDir := ""
				if s.Logs != nil {
					configuredDir = s.Logs.Dir
				}

				var baseLogDir string
				if configuredDir != "" {
					baseLogDir = configuredDir
				} else if runtime.GOOS != "windows" && os.Geteuid() == 0 {
					baseLogDir = paths.LogRoot
				} else if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
					baseLogDir = filepath.Join(stateHome, "lynx/logs")
				} else if home, err := os.UserHomeDir(); err == nil {
					baseLogDir = filepath.Join(home, ".local/state/lynx/logs")
				}

				if baseLogDir != "" {
					appLogDir = filepath.Join(baseLogDir, id)
				}
			}
		}

		if err := mgr.Delete(id); err != nil {
			auditEvent(auditor, ctx, "delete", id, delName, delNS, false, err)
			return nil, err
		}

		// Delete spec
		_ = spec.DeleteSpec(id) //nolint:errcheck // Ignore error if spec missing
		auditEvent(auditor, ctx, "delete", id, delName, delNS, true, nil)

		// Delete logs if requested
		if args.Purge && appLogDir != "" {
			base := appLogDir
			if idx := strings.LastIndex(appLogDir, string(os.PathSeparator)); idx != -1 {
				base = appLogDir[:idx]
			}
			baseResolved, err := filepath.EvalSymlinks(base)
			if err == nil {
				targetResolved, err := filepath.EvalSymlinks(appLogDir)
				if err == nil {
					rel, relErr := filepath.Rel(baseResolved, targetResolved)
					if relErr == nil && rel != ".." &&
						!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
						//nolint:gosec // path is validated to be within allowed log root
						_ = os.RemoveAll(
							targetResolved,
						)
					}
				}
			}
		}

		// Delete credentials if dynamic user
		credsDir := filepath.Join(paths.DataDir, "creds", id)
		_ = os.RemoveAll(credsDir)

		return jsonx.Marshal(map[string]string{"status": "deleted", "id": id})
	})

	server.Register("show", func(
		_ context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			return nil, err
		}

		if proc, ok := mgr.Get(id); ok {
			resp := map[string]any{
				"info": proc.Info(),
				"spec": proc.Spec(),
			}
			return jsonx.Marshal(resp)
		}

		s, err := spec.LoadSpec(id)
		if err != nil {
			return nil, fmt.Errorf("process not found: %s", args.ID)
		}

		resp := map[string]any{
			"spec": s,
		}
		return jsonx.Marshal(resp)
	})

	registerIDHandler(server, mgr, auditor, "reset", "reset", (*manager.Manager).Reset)
	registerIDHandler(server, mgr, auditor, "reload", "reloaded", (*manager.Manager).Reload)

	server.Register("scale", func(ctx context.Context, params jsonx.RawMessage) (jsonx.RawMessage, error) {
		var args struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Target    int    `json:"target"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		res, err := mgr.Scale(args.Namespace, args.Name, args.Target)
		if err != nil {
			auditEvent(auditor, ctx, "scale", args.Name, args.Name, args.Namespace, false, err)
			return nil, err
		}
		auditEvent(auditor, ctx, "scale", args.Name, args.Name, args.Namespace, true, nil)
		return jsonx.Marshal(res)
	})

	server.Register("flush", func(
		ctx context.Context,
		params jsonx.RawMessage,
	) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			auditEvent(auditor, ctx, "flush", args.ID, "", "", false, err)
			return nil, err
		}
		flushName, flushNS := processMeta(mgr, id)
		defer func() { auditEvent(auditor, ctx, "flush", id, flushName, flushNS, err == nil, err) }()

		var s *protocol.AppSpec
		if proc, ok := mgr.Get(id); ok {
			specCopy := proc.Spec()
			s = &specCopy
		} else {
			s, err = spec.LoadSpec(id)
			if err != nil {
				return nil, fmt.Errorf("process not found: %s", args.ID)
			}
		}

		var logsDir, stdout, stderr string
		if s.Logs != nil {
			logsDir = s.Logs.Dir
			stdout = s.Logs.Stdout
			stderr = s.Logs.Stderr
		}

		stdoutPath, stderrPath, err := paths.ResolveLogPaths(s.ID, logsDir, stdout, stderr)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve log paths: %w", err)
		}

		logRoot, err := paths.GetLogDir("")
		if s.Logs != nil && s.Logs.Dir != "" {
			logRoot, err = paths.GetLogDir(s.Logs.Dir)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to resolve log root: %w", err)
		}
		baseResolved, err := filepath.EvalSymlinks(logRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve log root symlinks: %w", err)
		}

		var bytesFreed int64
		for _, p := range []string{stdoutPath, stderrPath} {
			if p == "" {
				continue
			}

			targetPath := p
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(logRoot, targetPath)
			}
			targetPath = filepath.Clean(targetPath)

			targetDir := filepath.Dir(targetPath)
			targetResolvedDir, err := filepath.EvalSymlinks(targetDir)
			if err != nil {
				if os.IsNotExist(err) {
					dirClean := filepath.Clean(targetDir)
					relDir, relErr := filepath.Rel(baseResolved, dirClean)
					if relErr != nil || relDir == ".." ||
						strings.HasPrefix(relDir, ".."+string(os.PathSeparator)) {
						return nil, errors.New("refusing to truncate log outside log root")
					}
					relFile, relFileErr := filepath.Rel(baseResolved, targetPath)
					if relFileErr != nil || relFile == ".." ||
						strings.HasPrefix(relFile, ".."+string(os.PathSeparator)) {
						return nil, errors.New("refusing to truncate log outside log root")
					}
					continue
				}
				return nil, fmt.Errorf("failed to resolve log directory symlinks: %w", err)
			}

			relDir, relErr := filepath.Rel(baseResolved, targetResolvedDir)
			if relErr != nil || relDir == ".." ||
				strings.HasPrefix(relDir, ".."+string(os.PathSeparator)) {
				return nil, errors.New("refusing to truncate log outside log root")
			}

			relFile, relFileErr := filepath.Rel(baseResolved, targetPath)
			if relFileErr != nil || relFile == ".." ||
				strings.HasPrefix(relFile, ".."+string(os.PathSeparator)) {
				return nil, errors.New("refusing to truncate log outside log root")
			}

			info, err := os.Lstat(targetPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("failed to stat log file: %w", err)
			}

			if info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("ERR_BAD_REQUEST: refusing to truncate symlink log file")
			}

			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("refusing to truncate non-regular log file %s", targetPath)
			}

			sizeBefore := info.Size()
			if err := os.Truncate(targetPath, 0); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to truncate %s: %w", targetPath, err)
			}
			bytesFreed += sizeBefore
		}

		return jsonx.Marshal(map[string]any{"status": "flushed", "id": id, "bytes_freed": bytesFreed})
	})

	// Register list handler (replacing status)
	// Returns a list of processes with their detailed status
	server.Register("list", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(mgr.List())
	})

	// Register version handler
	server.Register(
		"version",
		func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
			return jsonx.Marshal(version.Get())
		},
	)
}

// registerIDHandler wires a verb whose request is {id} and response is
// {status, id}. Used for the simple id-in / action-out verbs: restart,
// reload, reset. Flush/delete/stop do extra per-verb work and stay open-coded.
func registerIDHandler(
	server *transport.Server,
	mgr *manager.Manager,
	auditor *audit.Logger,
	verb, pastTense string,
	action func(*manager.Manager, string) error,
) {
	server.Register(verb, func(ctx context.Context, params jsonx.RawMessage) (jsonx.RawMessage, error) {
		var args struct {
			ID string `json:"id"`
		}
		if err := jsonx.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		id, err := mgr.ResolveID(args.ID)
		if err != nil {
			auditEvent(auditor, ctx, verb, id, "", "", false, err)
			return nil, err
		}
		if err := action(mgr, id); err != nil {
			auditEvent(auditor, ctx, verb, id, "", "", false, err)
			return nil, err
		}
		name, ns := processMeta(mgr, id)
		auditEvent(auditor, ctx, verb, id, name, ns, true, nil)
		return jsonx.Marshal(map[string]string{"status": pastTense, "id": id})
	})
}

// auditEvent populates caller identity from ctx and forwards to the logger.
// Safe to call with a Disabled logger.
func auditEvent(l *audit.Logger, ctx context.Context, action, target, name, ns string, ok bool, err error) {
	e := audit.Event{
		Action:  action,
		Target:  target,
		Name:    name,
		NS:      ns,
		Success: ok,
	}
	if err != nil {
		e.Error = err.Error()
	}
	if id, okc := ctx.Value(transport.ContextKeyIdentity).(*transport.Identity); okc && id != nil {
		e.UID = id.UID
		e.GID = id.GID
		e.PID = id.PID
	}
	l.Log(e)
}

// processMeta best-effort fetches name+namespace for audit enrichment. Empty
// strings if the process is already gone (e.g. post-delete).
func processMeta(mgr *manager.Manager, id string) (name, ns string) {
	if p, ok := mgr.Get(id); ok {
		info := p.Info()
		return info.Name, info.Namespace
	}
	return "", ""
}
