package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Info holds Git repository information.
type Info struct {
	Branch string
	Commit string
	Dirty  bool
}

// GetInfo retrieves Git information from the given directory.
// It returns empty strings if the directory is not a Git repository or git is not installed.
func GetInfo(dir string) (Info, error) {
	info := Info{}

	// Check if git is installed
	if _, err := exec.LookPath("git"); err != nil {
		return info, nil
	}

	// Check if .git directory exists directly in dir
	// This prevents picking up parent git repositories
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return info, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Get Branch
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil {
		info.Branch = strings.TrimSpace(string(out))
	} else {
		// Maybe detached HEAD?
		info.Branch = "detached"
	}

	// Get Commit Hash
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err = cmd.Output()
	if err == nil {
		info.Commit = strings.TrimSpace(string(out))
	}

	// Check for uncommitted changes
	if checkDirty(ctx, dir) {
		info.Dirty = true
	}

	return info, nil
}

func checkDirty(ctx context.Context, dir string) bool {
	// git diff --quiet returns 1 if there are changes
	cmd := exec.CommandContext(ctx, "git", "diff", "--quiet")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true
		}
	}

	// Check for staged changes
	cmd = exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true
		}
	}
	return false
}
