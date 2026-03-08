// Package update implements the update command.
package update

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/updater"
	"github.com/Jaro-c/Lynx/internal/version"
)

// Run executes the update command.
func Run(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	apply := fs.Bool("apply", false, "Apply the update if available")
	_ = fs.Bool("check", true, "Check for updates (default)")
	force := fs.Bool("force", false, "Force update even if managed by system package manager")

	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	if err := fs.Parse(args); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			flagName := strings.TrimPrefix(err.Error(), "flag provided but not defined: -")
			return &errs.UsageError{Message: "Unknown flag: -" + flagName}
		}
		return &errs.UsageError{Message: err.Error()}
	}

	if len(fs.Args()) > 0 {
		return &errs.UsageError{Message: fmt.Sprintf("Unexpected arguments: %v", fs.Args())}
	}

	// 1. Check if managed by system package manager
	isManaged := updater.IsManagedByPackageSystem()
	if isManaged && *apply && !*force {
		return errors.New("lynx is managed by system package manager (dpkg). Please download the latest .deb release and install it using 'sudo apt install ./lynx_<version>_amd64.deb'. Use --force to override (not recommended)")
	}

	fmt.Fprintf(w, "Checking for updates...\n")

	// 2. Check for updates
	release, err := updater.Check(context.Background())
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if release == nil {
		fmt.Fprintf(w, "%s You are using the latest version (%s)\n", term.GreenString("✓"), version.Version)
		return nil
	}

	fmt.Fprintf(w, "%s New version available: %s\n", term.YellowString("!"), term.BoldString("%s", release.TagName))
	fmt.Fprintf(w, "  Release notes: %s\n", release.HTMLURL)

	// 3. Apply update if requested
	if *apply {
		fmt.Fprintf(w, "Downloading and installing update...\n")
		if err := updater.Apply(context.Background(), release); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		fmt.Fprintf(w, "%s Successfully updated to %s\n", term.GreenString("✓"), release.TagName)
		fmt.Fprintf(w, "Please restart the daemon manually if needed: 'systemctl restart lynx.lynxd' or 'lynx reload'\n")
	} else {
		if isManaged {
			fmt.Fprintf(w, "\nTo update, download the latest .deb release from %s\n", release.HTMLURL)
			fmt.Fprintf(w, "and run:\n  sudo apt install ./<downloaded_deb_file>\n")
		} else {
			fmt.Fprintf(w, "\nTo update, run:\n  lynx update --apply\n")
		}
	}

	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "update",
		Usage:       term.BoldString("lynx update [flags]"),
		Description: "Check for updates and apply them.",
		Options: []help.Option{
			{Short: "-a", Long: "--apply", Description: "Download and apply the update."},
			{Short: "-c", Long: "--check", Description: "Check for updates without applying (default)."},
			{Short: "-f", Long: "--force", Description: "Force update even if managed by system package manager."},
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message for the update command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
