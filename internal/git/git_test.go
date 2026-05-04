package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/git"
)

func TestGetInfo(t *testing.T) {
	// Check if git is installed
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed, skipping test")
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "lynx-git-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Case 1: Not a git repo
	info, err := git.GetInfo(tempDir)
	if err != nil {
		t.Fatalf("GetInfo failed on non-git repo: %v", err)
	}
	if info.Branch != "" || info.Commit != "" {
		t.Errorf("Expected empty info for non-git repo, got: %+v", info)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize git repo
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user (needed for commit)
	// We need to set these config values locally to avoid errors in environments without global git config
	_ = exec.CommandContext(
		ctx,
		"git",
		"-C",
		tempDir,
		"config",
		"user.email",
		"test@example.com",
	).Run()
	_ = exec.CommandContext(
		ctx,
		"git",
		"-C",
		tempDir,
		"config",
		"user.name",
		"Test User",
	).Run()

	// Default branch name to main to avoid differences between git versions
	_ = exec.CommandContext(
		ctx,
		"git",
		"-C",
		tempDir,
		"checkout",
		"-b",
		"main",
	).Run()

	// Create a commit
	if err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.CommandContext(ctx, "git", "add", "test.txt")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial commit")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create a second commit to verify we get the latest
	if err := os.WriteFile(
		filepath.Join(tempDir, "test.txt"),
		[]byte("hello world"),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	cmd = exec.CommandContext(ctx, "git", "add", "test.txt")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add 2 failed: %v", err)
	}
	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "second commit")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit 2 failed: %v", err)
	}

	// Case 3: Valid git repo
	info, err = git.GetInfo(tempDir)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if info.Branch == "" {
		t.Error("Expected branch, got empty")
	}
	if info.Commit == "" {
		t.Error("Expected commit, got empty")
	}
	if info.Dirty {
		t.Error("Expected clean repo, got dirty")
	}

	// Case 4: Dirty repo
	if err := os.WriteFile(
		filepath.Join(tempDir, "test.txt"),
		[]byte("changed"),
		0600,
	); err != nil {
		t.Fatal(err)
	}
	info, err = git.GetInfo(tempDir)
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if !info.Dirty {
		t.Error("Expected dirty repo, got clean")
	}
}
