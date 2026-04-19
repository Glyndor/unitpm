// Package update implements the update command.
package update

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
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
	insecureSkipSig := fs.Bool("insecure-skip-signature", false, "Accept unsigned releases (dangerous)")

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
		return errors.New(
			"lynxpm is managed by system package manager (dpkg). " +
				"Please download the latest .deb release and install it using " +
				"'sudo apt install ./lynx_<version>_amd64.deb'. " +
				"Use --force to override (not recommended)",
		)
	}

	_, _ = fmt.Fprintf(w, "Checking for updates...\n")

	// 2. Check for updates
	release, err := updater.Check(context.Background())
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if release == nil {
		_, _ = fmt.Fprintf(
			w,
			"%s You are using the latest version (%s)\n",
			term.GreenString("✓"),
			version.Version,
		)
		return nil
	}

	_, _ = fmt.Fprintf(
		w,
		"%s New version available: %s\n",
		term.YellowString("!"),
		term.BoldString("%s", release.TagName),
	)
	_, _ = fmt.Fprintf(w, "  Release notes: %s\n", release.HTMLURL)

	// 3. Apply update if requested
	if *apply {
		_, _ = fmt.Fprintf(w, "Downloading and installing update...\n")
		if err := updater.Apply(context.Background(), release, updater.ApplyOptions{
			AllowUnsigned: *insecureSkipSig,
		}); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		_, _ = fmt.Fprintf(w, "%s Successfully updated to %s\n", term.GreenString("✓"), release.TagName)
		_, _ = fmt.Fprintf(
			w,
			"Please restart the daemon manually if needed: 'systemctl restart lynxd' or 'lynxpm reload'\n",
		)
	} else {
		if isManaged {
			debURL := findDebAsset(release)
			if debURL != "" {
				debFile := debURL[strings.LastIndex(debURL, "/")+1:]
				_, _ = fmt.Fprintf(w, "\nTo update, run:\n")
				_, _ = fmt.Fprintf(w, "  wget %s\n", debURL)
				_, _ = fmt.Fprintf(w, "  sudo apt install ./%s\n", debFile)
			} else {
				_, _ = fmt.Fprintf(
					w,
					"\nTo update, download the latest .deb release from %s\n",
					release.HTMLURL,
				)
				_, _ = fmt.Fprintf(w, "and run:\n  sudo apt install ./<downloaded_deb_file>\n")
			}
		} else {
			_, _ = fmt.Fprintf(w, "\nTo update, run:\n  lynxpm update --apply\n")
		}
	}

	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "update",
		Usage:       term.BoldString("lynxpm update [flags]"),
		Description: "Check for updates and apply them.",
		Options: []help.Option{
			{Short: "-a", Long: "--apply", Description: "Download and apply the update."},
			{
				Short:       "-c",
				Long:        "--check",
				Description: "Check for updates without applying (default).",
			},
			{
				Short:       "-f",
				Long:        "--force",
				Description: "Force update even if managed by system package manager.",
			},
			{
				Short:       "",
				Long:        "--insecure-skip-signature",
				Description: "Accept unsigned releases. Dangerous: skips integrity/authenticity verification.",
			},
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

func findDebAsset(release *updater.Release) string {
	arch := runtime.GOARCH
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".deb") && strings.Contains(asset.Name, arch) {
			return asset.BrowserDownloadURL
		}
	}
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".deb") {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

// PrintHelp prints the help message for the update command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
