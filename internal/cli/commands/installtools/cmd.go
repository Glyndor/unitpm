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

// Tools to check and link.
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

	autoYes := false
	systemMode := false
	for _, arg := range args {
		switch arg {
		case "-y", "--yes":
			autoYes = true
		case "--system":
			systemMode = true
		}
	}

	var destDir string
	if systemMode {
		// System-wide install requires root
		if os.Geteuid() != 0 {
			return fmt.Errorf("--system requires root privileges (run with sudo)")
		}
		destDir = "/usr/local/bin"
	} else {
		// User install: ~/.local/bin — no sudo needed
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to determine home directory: %w", err)
		}
		destDir = filepath.Join(home, ".local", "bin")
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", destDir, err)
		}
	}

	// In system mode, use runuser to find tools in the original user's PATH.
	// In user mode, use the current process PATH directly.
	sudoUser := ""
	if systemMode {
		sudoUser = os.Getenv("SUDO_USER")
		if sudoUser == "" {
			_, _ = term.Println(term.YellowString("! SUDO_USER not set. Scanning root's PATH only."))
			_, _ = term.Println("  Ideally run as: sudo lynx install-tools --system")
		}
	}

	_, _ = term.Printf("Scanning for development tools to link to %s...\n", destDir)

	type plannedLink struct {
		Tool string
		Src  string
		Dest string
	}
	var plan []plannedLink

	for _, tool := range commonTools {
		destPath := filepath.Join(destDir, tool)
		if _, err := os.Lstat(destPath); err == nil {
			// Already exists in dest
			continue
		}

		var srcPath string
		var err error
		if systemMode && sudoUser != "" {
			srcPath, err = findViaRunuser(sudoUser, tool)
		} else {
			srcPath, err = exec.LookPath(tool)
		}
		if err != nil || srcPath == "" {
			continue
		}

		// Prevent circular symlinks
		if resolved, err := filepath.EvalSymlinks(srcPath); err == nil {
			if resolved == destPath {
				_, _ = term.Printf("%s Skipping %s: resolves to destination (loop)\n", term.YellowString("!"), tool)
				continue
			}
		}

		plan = append(plan, plannedLink{
			Tool: tool,
			Src:  srcPath,
			Dest: destPath,
		})
	}

	if len(plan) == 0 {
		_, _ = term.Printf("%s No new tools found to link. Everything up to date.\n", term.GreenString("✓"))
		return nil
	}

	_, _ = term.Println(term.BoldString("\nPlan of execution:"))
	for _, p := range plan {
		_, _ = term.Printf("  %s %s -> %s\n", term.GreenString("+"), term.CyanString("%s", p.Tool), p.Src)
	}
	_, _ = term.Println()

	if !autoYes {
		fmt.Printf("Proceed with linking %d tools to %s? [Y/n/choose] ", len(plan), destDir)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "n" || response == "no" {
			fmt.Println("Aborted.")
			return nil
		}

		if response == "choose" || response == "c" {
			var filtered []plannedLink
			for _, p := range plan {
				fmt.Printf("Link %s (%s)? [Y/n] ", term.CyanString("%s", p.Tool), p.Src)
				ans, _ := reader.ReadString('\n')
				ans = strings.TrimSpace(strings.ToLower(ans))
				if ans != "n" && ans != "no" {
					filtered = append(filtered, p)
				}
			}
			plan = filtered
			if len(plan) == 0 {
				fmt.Println("Nothing selected. Aborting.")
				return nil
			}
		}
	}

	count := 0
	for _, p := range plan {
		_, _ = term.Printf("  Linking %s... ", p.Tool)
		if err := os.Symlink(p.Src, p.Dest); err != nil {
			_, _ = term.Printf("%s %v\n", term.RedString("✗"), err)
			continue
		}
		_, _ = term.Println(term.GreenString("✓"))
		count++
	}

	_, _ = term.Printf("\n%s Linked %d tools to %s\n", term.GreenString("✓"), count, destDir)

	if !systemMode {
		path := os.Getenv("PATH")
		home, _ := os.UserHomeDir()
		localBin := filepath.Join(home, ".local", "bin")
		if !strings.Contains(path, localBin) {
			_, _ = term.Printf("\n%s Add %s to your PATH:\n", term.YellowString("!"), localBin)
			_, _ = term.Printf("  echo 'export PATH=\"%s:$PATH\"' >> ~/.bashrc\n", localBin)
		}
	}

	return nil
}

// findViaRunuser locates a tool binary via a full login shell for the given user.
// Used in --system mode so we can find tools installed in the original user's PATH.
func findViaRunuser(user, tool string) (string, error) {
	cmd := exec.Command("runuser", "-l", user, "-c", "which "+tool)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("not found")
	}
	return path, nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "install-tools",
		Usage:       term.BoldString("lynx install-tools [options]"),
		Description: "Symlink common dev tools (bun, node, go, etc.) into ~/.local/bin " +
			"so the Lynx daemon can find them. Use --system for a system-wide install.",
		Options: []help.Option{
			{Short: "", Long: "--system", Description: "Install to /usr/local/bin instead (requires sudo)"},
			{Short: "-y", Long: "--yes", Description: "Automatically confirm all prompts"},
			{Short: "-h", Long: "--help", Description: "Show this help message"},
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
