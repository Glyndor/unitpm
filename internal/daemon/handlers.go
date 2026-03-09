//nolint:gocognit,cyclop,nestif,gocyclo,funlen
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

// RegisterHandlers registers all daemon IPC handlers.
func RegisterHandlers(server *transport.Server, mgr *manager.Manager, privileged bool) {
	// Register ping handler
	server.Register("ping", func(_ context.Context, _ jsonx.RawMessage) (jsonx.RawMessage, error) {
		return jsonx.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler
	server.Register("start", handlers.StartHandler(mgr, privileged))

	// Register stop handler
	server.Register("stop", func(
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

		if err := mgr.Stop(id); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "stopped", "id": id})
	})

	// Register restart handler
	server.Register("restart", func(
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

		if err := mgr.Restart(id); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "restarted", "id": id})
	})

	// Register delete handler
	server.Register("delete", func(
		_ context.Context,
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
			return nil, err
		}

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
			return nil, err
		}

		// Delete spec
		_ = spec.DeleteSpec(id) //nolint:errcheck // Ignore error if spec missing

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

	server.Register("reload", func(
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

		if err := mgr.Reload(id); err != nil {
			return nil, err
		}

		return jsonx.Marshal(map[string]string{"status": "reloaded", "id": id})
	})

	server.Register("flush", func(
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

			if err := os.Truncate(targetPath, 0); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to truncate %s: %w", targetPath, err)
			}
		}

		return jsonx.Marshal(map[string]string{"status": "flushed", "id": id})
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
