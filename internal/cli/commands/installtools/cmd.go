//go:build linux

// Package installtools implements the install-tools command.
package installtools

import (
	"bufio"
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

	// Check for auto-yes flag
	autoYes := false
	for _, arg := range args {
		if arg == "-y" || arg == "--yes" {
			autoYes = true
		}
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

	type plannedLink struct {
		Tool string
		Src  string
		Dest string
	}
	var plan []plannedLink

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

		// Prevent circular symlinks:
		// If userPath resolves to globalPath, we shouldn't link it.
		// Also check if userPath resolves to the same file as globalPath (if globalPath existed)
		// Since globalPath doesn't exist (checked above), we mainly check if userPath points to globalPath
		// or if we are linking a file to itself in a way that causes a loop.
		
		resolvedUserPath, err := filepath.EvalSymlinks(userPath)
		if err == nil {
			if resolvedUserPath == globalPath {
				fmt.Printf("Skipping %s: resolves to destination %s (circular loop detected)\n", tool, globalPath)
				continue
			}
		}

		plan = append(plan, plannedLink{
			Tool: tool,
			Src:  userPath,
			Dest: globalPath,
		})
	}

	if len(plan) == 0 {
		fmt.Println("No new tools found to link. Everything seems up to date.")
		return nil
	}

	// Display plan
	fmt.Println(term.BoldString("\nPlan of execution:"))
	for _, p := range plan {
		fmt.Printf("  %s %s -> %s\n", term.GreenString("+"), term.CyanString("%s", p.Tool), p.Src)
	}
	fmt.Println()

	// Confirm
	if !autoYes {
		fmt.Printf("Do you want to proceed with linking %d tools? [Y/n/choose] ", len(plan))
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "n" || response == "no" {
			fmt.Println("Operation aborted by user.")
			return nil
		}

		if response == "choose" || response == "c" {
			var filteredPlan []plannedLink
			for _, p := range plan {
				fmt.Printf("Link %s (%s)? [Y/n] ", term.CyanString("%s", p.Tool), p.Src)
				ans, _ := reader.ReadString('\n')
				ans = strings.TrimSpace(strings.ToLower(ans))
				if ans != "n" && ans != "no" {
					filteredPlan = append(filteredPlan, p)
				}
			}
			plan = filteredPlan
			if len(plan) == 0 {
				fmt.Println("No tools selected. Aborting.")
				return nil
			}
		}
	}

	// Execute
	count := 0
	for _, p := range plan {
		fmt.Printf("Linking %s... ", p.Tool)
		if err := os.Symlink(p.Src, p.Dest); err != nil {
			fmt.Print(term.RedString("Failed: %v\n", err))
			continue
		}
		fmt.Println("Done")
		count++
	}

	fmt.Print(term.GreenString("\nSuccessfully linked %d tools to /usr/local/bin\n", count))
	return nil
}

// findUserTool tries to locate a binary as the sudo user
func findUserTool(user, tool string) (string, error) {
	if user == "" {
		return exec.LookPath(tool)
	}

	// Use 'runuser -l' to simulate a full login shell, ensuring .bashrc/.zshrc
	// and the user's full PATH are loaded (crucial for tools like Bun in ~/.bun/bin)
	cmd := exec.Command("runuser", "-l", user, "-c", "which "+tool)
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
		Usage:       term.BoldString("sudo lynx install-tools [options]"),
		Description: "Automatically symlink common dev tools (bun, node, go, etc) to /usr/local/bin so Lynx daemon can find them.",
		Options: []help.Option{
			{Short: "-y", Long: "--yes", Description: "Automatically confirm all prompts."},
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
