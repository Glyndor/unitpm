//go:build linux

// Package installtools implements the install-tools command.
package installtools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Tools to check and link
var commonTools = []string{
	"bun", "node", "npm", "pnpm", "yarn",
	"go", "python", "python3", "pip", "pip3",
	"ruby", "gem", "rustc", "cargo",
	"java", "javac", "deno",
}

// Run executes the install-tools command.
func Run(args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	// Must be root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges (run with sudo)")
	}

	// Get original user from SUDO_USER env var if available, to scan their home dir
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		fmt.Println(term.YellowString("Warning: SUDO_USER not set. Scanning root's PATH only."))
		fmt.Println("Ideally, run this as: sudo lynx install-tools")
	}

	// We want to find where these tools are for the *user*, not necessarily root.
	// However, `exec.LookPath` uses current PATH. When running with sudo, PATH might be restricted.
	// Strategy:
	// 1. Check if tool exists in /usr/local/bin (already linked or installed global). If yes, skip.
	// 2. If not, try to find it in the user's likely paths or asking `which` as the user.

	fmt.Println("Scanning for development tools to link globally...")
	count := 0

	for _, tool := range commonTools {
		// 1. Check if already global
		globalPath := filepath.Join("/usr/local/bin", tool)
		if _, err := os.Lstat(globalPath); err == nil {
			// Already exists
			continue
		}

		// 2. Find the tool path for the user
		userPath, err := findUserTool(sudoUser, tool)
		if err != nil {
			continue // Not found
		}

		if userPath == "" {
			continue
		}

		// 3. Create symlink
		fmt.Printf("Linking %s -> %s\n", tool, userPath)
		if err := os.Symlink(userPath, globalPath); err != nil {
			fmt.Printf(term.RedString("  Failed to link %s: %v\n", tool, err))
			continue
		}
		count++
	}

	if count == 0 {
		fmt.Println("No new tools linked. Everything seems up to date or not found.")
	} else {
		fmt.Printf(term.GreenString("Successfully linked %d tools to /usr/local/bin\n"), count)
	}

	return nil
}

// findUserTool tries to locate a binary as the sudo user
func findUserTool(user, tool string) (string, error) {
	if user == "" {
		return exec.LookPath(tool)
	}

	// Use 'su -c which' to find the path as the user
	// This respects the user's PATH configuration in .bashrc/.zshrc etc (mostly)
	cmd := exec.Command("runuser", "-u", user, "--", "which", tool)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("not found")
	}

	// Validate it's an absolute path
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid path returned")
	}

	return path, nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "install-tools",
		Usage:       term.BoldString("sudo lynx install-tools"),
		Description: "Automatically symlink common dev tools (bun, node, go, etc) to /usr/local/bin so Lynx daemon can find them.",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
