// Package paths resolves XDG-aware filesystem paths (config, logs,
// runtime socket) for both system and user mode deployments.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// System-mode filesystem layout. User-mode overrides these with XDG paths.
const (
	// LogRoot is the system-wide directory where lynxd writes per-process logs.
	LogRoot = "/var/log/lynx-pm"
	// RunDir is the system-mode runtime directory that holds the IPC socket.
	RunDir = "/run/lynxd"
	// CredsDir is where systemd LoadCredential= staging files are written
	// for --isolation dynamic (one subdirectory per process ID).
	CredsDir = "/var/lib/lynx-pm/creds"
	// DataDir is the persistent state root for the lynx system user.
	DataDir = "/var/lib/lynx-pm"
)

var currentEuid = getEuid

// IsRoot reports whether the current process is running as root (euid 0).
func IsRoot() bool {
	return currentEuid() == 0
}

// GetLogDir resolves the root log directory.
func GetLogDir(configuredDir string) (string, error) {
	if configuredDir != "" {
		return resolveConfiguredDir(configuredDir)
	}
	return resolveDefaultDir()
}

func resolveConfiguredDir(configuredDir string) (string, error) {
	if len(configuredDir) > 4096 {
		return "", errors.New("log dir too long")
	}
	clean := filepath.Clean(configuredDir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid log dir")
	}

	if IsSystemMode() {
		return resolveRootLogDir(clean)
	}

	return clean, nil
}

func resolveRootLogDir(candidate string) (string, error) {
	if !filepath.IsAbs(candidate) {
		return "", errors.New("invalid log dir: must be absolute in system mode")
	}

	allowedRoots := []string{LogRoot}
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		allowedRoots = append(allowedRoots, filepath.Join(stateHome, "lynx/logs"))
	}

	// Resolve each allowed root once up front so comparisons work even when
	// the roots themselves are symlinks (e.g. /var -> /private/var on macOS).
	resolvedRoots := make([]string, 0, len(allowedRoots))
	for _, root := range allowedRoots {
		base := filepath.Clean(root)
		if !filepath.IsAbs(base) {
			continue
		}
		if r, err := filepath.EvalSymlinks(base); err == nil {
			base = r
		}
		resolvedRoots = append(resolvedRoots, base)
	}

	for _, root := range resolvedRoots {
		if !WithinRoot(root, candidate) {
			continue
		}

		if matchResolvedRoot(root, candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("invalid log dir: outside allowed roots")
}

// matchResolvedRoot reports whether candidate is safely within root.
// When the candidate exists we resolve it and compare; when it does not exist
// yet (pre-create check) we fall back to scanning each path component for
// symlinks that escape the root — preventing a TOCTOU race where a symlink is
// planted between the check and the first write.
func matchResolvedRoot(root, candidate string) bool {
	if candidateResolved, err := filepath.EvalSymlinks(candidate); err == nil {
		return WithinRoot(root, candidateResolved)
	} else if !os.IsNotExist(err) {
		return false
	}

	return WithinRoot(root, candidate) && !pathContainsUnsafeSymlink(root, candidate)
}

func resolveDefaultDir() (string, error) {
	if IsSystemMode() {
		return LogRoot, nil
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome != "" {
		return filepath.Join(stateHome, "lynx/logs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home: %w", err)
	}
	return filepath.Join(home, ".local/state/lynx/logs"), nil
}

// WithinRoot reports whether path resolves inside root (no .. escape).
func WithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func pathContainsUnsafeSymlink(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	current := root
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		// #nosec G703 // path is fully sanitized component by component during resolution
		info, err := os.Lstat(current)
		if err != nil {
			return !os.IsNotExist(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return true
			}
			if !WithinRoot(root, resolved) {
				return true
			}
		}
	}
	return false
}

// ResolveLogPaths returns the absolute paths for stdout and stderr logs for a given spec.
func ResolveLogPaths(specID, logsDir, stdout, stderr string) (string, string, error) {
	logDir, err := GetLogDir(logsDir)
	if err != nil {
		return "", "", err
	}

	// Per-app log directory
	appLogDir := filepath.Join(logDir, specID)

	// Stdout
	stdoutPath := stdout
	if stdoutPath == "" {
		stdoutPath = "stdout.log"
	}
	if !filepath.IsAbs(stdoutPath) {
		stdoutPath = filepath.Join(appLogDir, stdoutPath)
	}

	// Stderr
	stderrPath := stderr
	if stderrPath == "" {
		stderrPath = "stderr.log"
	}
	if !filepath.IsAbs(stderrPath) {
		stderrPath = filepath.Join(appLogDir, stderrPath)
	}

	return stdoutPath, stderrPath, nil
}
