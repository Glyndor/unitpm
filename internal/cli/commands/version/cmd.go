// Package version implements the version command.
package version

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/version"
)

// Run executes the version command.
func Run(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

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

	local := version.Get()

	// 1. Print local CLI version
	fmt.Fprintf(w, "%s\n", term.CyanString("%s", term.BoldString("Lynx CLI")))
	printVersionInfo(w, local)

	// 2. Attempt to connect to daemon
	// We handle the connection manually here because failure is not an error for this command.
	client, err := ipc.NewClient()
	if err != nil {
		// Daemon not running or unreachable.
		// Print only Protocol section and exit 0.
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
		fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("CLI"),
			term.BoldString("v%d", local.ProtocolVersion),
		)
		return nil
	}
	defer client.Close()

	// 3. Fetch daemon version
	var daemonInfo version.Info
	err = client.Call("version", nil, &daemonInfo)
	if err != nil {
		if handleProtocolMismatch(w, local, err) {
			return errors.New("protocol mismatch")
		}

		// Other errors (e.g. timeout, or daemon internal error)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
		fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("CLI"),
			term.BoldString("v%d", local.ProtocolVersion),
		)
		return nil
	}

	// 4. Print daemon version
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", term.CyanString("%s", term.BoldString("Lynx Daemon")))
	printVersionInfo(w, daemonInfo)

	// 5. Print Protocol
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
	fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("CLI"),
		term.BoldString("v%d", local.ProtocolVersion),
	)
	fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("Daemon"),
		term.BoldString("v%d", daemonInfo.ProtocolVersion),
	)

	return nil
}

func handleProtocolMismatch(w io.Writer, local version.Info, err error) bool {
	var remoteErr *ipc.RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.Code != "PROTOCOL_MISMATCH" {
		return false
	}

	// Extract supported version safely using typed struct
	var supported int
	if data, ok := remoteErr.Data.(ipc.ProtocolMismatchData); ok {
		supported = data.Supported
	}

	// Print Protocol section with error details
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
	fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("CLI"),
		term.BoldString("v%d", local.ProtocolVersion),
	)
	if supported > 0 {
		fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("Daemon"),
			term.BoldString("v%d", supported),
		)
	} else {
		fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("Daemon"),
			term.BoldString("unknown"),
		)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s\n", term.RedString("Error: Protocol mismatch"))
	if supported > 0 {
		fmt.Fprintf(
			w,
			"The CLI (v%d) and Daemon (v%d) have incompatible protocols.\n",
			local.ProtocolVersion,
			supported,
		)
	} else {
		fmt.Fprintf(
			w,
			"The CLI (v%d) and Daemon have incompatible protocols.\n",
			local.ProtocolVersion,
		)
	}

	// Return true to indicate handled
	return true
}

func printVersionInfo(w io.Writer, info version.Info) {
	fmt.Fprintf(w, "  %s : %s\n", term.DimString("Version"), term.BoldString("%s", info.Version))
	fmt.Fprintf(w, "  %s : %s\n", term.DimString("Commit"), term.BoldString("%s", info.Commit))
	fmt.Fprintf(w, "  %s : %s\n", term.DimString("Built"), term.BoldString("%s", info.BuildDate))
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "version",
		Usage:       term.BoldString("lynx version"),
		Description: "Show version information for CLI and Daemon.",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message for the version command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
