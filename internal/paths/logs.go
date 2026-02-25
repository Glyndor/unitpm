package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

var getEuid = os.Geteuid

// GetLogDir resolves the root log directory.
func GetLogDir(configuredDir string) (string, error) {
	euid := getEuid()

	if configuredDir != "" {
		return resolveConfiguredDir(configuredDir, euid)
	}
	return resolveDefaultDir(euid)
}

func resolveConfiguredDir(configuredDir string, euid int) (string, error) {
	if len(configuredDir) > 4096 {
		return "", errors.New("log dir too long")
	}
	clean := filepath.Clean(configuredDir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid log dir")
	}

	if euid == 0 {
		return resolveRootLogDir(clean)
	}

	return clean, nil
}

func resolveRootLogDir(candidate string) (string, error) {
	if !filepath.IsAbs(candidate) {
		return "", errors.New("invalid log dir: must be absolute when running as root")
	}

	allowedRoots := []string{"/var/log/lynx"}
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		allowedRoots = append(allowedRoots, filepath.Join(stateHome, "lynx/logs"))
	}

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
		if !withinRoot(root, candidate) {
			continue
		}

		if matchResolvedRoot(root, candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("invalid log dir: outside allowed roots")
}

func matchResolvedRoot(root, candidate string) bool {
	if candidateResolved, err := filepath.EvalSymlinks(candidate); err == nil {
		return withinRoot(root, candidateResolved)
	} else if !os.IsNotExist(err) {
		// Some error other than IsNotExist
		return false
	}

	return withinRoot(root, candidate) && !pathContainsUnsafeSymlink(root, candidate)
}

func resolveDefaultDir(euid int) (string, error) {
	if euid == 0 {
		return "/var/log/lynx", nil
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

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
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
		info, err := os.Lstat(current)
		if err != nil {
			return !os.IsNotExist(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return true
			}
			if !withinRoot(root, resolved) {
				return true
			}
		}
	}
	return false
}

// ResolveLogPaths returns the absolute paths for stdout and stderr logs for a given spec.
func ResolveLogPaths(spec *protocol.AppSpec) (string, string, error) {
	logs := spec.Logs
	if logs == nil {
		logs = &protocol.AppLogs{}
	}

	logDir, err := GetLogDir(logs.Dir)
	if err != nil {
		return "", "", err
	}

	// Per-app log directory
	appLogDir := filepath.Join(logDir, spec.ID)

	// Stdout
	stdoutPath := logs.Stdout
	if stdoutPath == "" {
		stdoutPath = "stdout.log"
	}
	if !filepath.IsAbs(stdoutPath) {
		stdoutPath = filepath.Join(appLogDir, stdoutPath)
	}

	// Stderr
	stderrPath := logs.Stderr
	if stderrPath == "" {
		stderrPath = "stderr.log"
	}
	if !filepath.IsAbs(stderrPath) {
		stderrPath = filepath.Join(appLogDir, stderrPath)
	}

	return stdoutPath, stderrPath, nil
}
