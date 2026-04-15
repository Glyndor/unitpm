// Package version implements the version command: reports CLI + daemon versions and the IPC protocol version.
package version

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/version"
)

// Run executes the version command.
// client is optional; if nil, it attempts to connect to the daemon.
func Run(client transport.IPCClient, w io.Writer, args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Output version info as JSON")

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

	local := version.Get()

	// 2. Attempt to connect to daemon
	var err error
	if client == nil {
		client, err = transport.NewClient()
	}

	var daemonInfo *version.Info
	var daemonErr error
	if err == nil {
		defer func() { _ = client.Close() }()
		var di version.Info
		daemonErr = client.Call("version", nil, &di)
		if daemonErr == nil {
			daemonInfo = &di
		}
	}

	// JSON output mode
	if jsonOutput {
		type versionEntry struct {
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildDate string `json:"build_date"`
		}
		type protocolEntry struct {
			CLI    int  `json:"cli"`
			Daemon *int `json:"daemon,omitempty"`
		}
		type jsonOutput struct {
			CLI      versionEntry   `json:"cli"`
			Daemon   *versionEntry  `json:"daemon,omitempty"`
			Protocol protocolEntry  `json:"protocol"`
		}

		out := jsonOutput{
			CLI: versionEntry{
				Version:   local.Version,
				Commit:    local.Commit,
				BuildDate: local.BuildDate,
			},
			Protocol: protocolEntry{
				CLI: local.ProtocolVersion,
			},
		}
		if daemonInfo != nil {
			out.Daemon = &versionEntry{
				Version:   daemonInfo.Version,
				Commit:    daemonInfo.Commit,
				BuildDate: daemonInfo.BuildDate,
			}
			dv := daemonInfo.ProtocolVersion
			out.Protocol.Daemon = &dv
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// 1. Print local CLI version
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("%s", term.BoldString("Lynx CLI")))
	printVersionInfo(w, local)

	if err != nil {
		// Daemon not running or unreachable.
		// Print only Protocol section and exit 0.
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
		_, _ = fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("CLI"),
			term.BoldString("v%d", local.ProtocolVersion),
		)
		return nil
	}

	if daemonErr != nil {
		if handleProtocolMismatch(w, local, daemonErr) {
			return errors.New("protocol mismatch")
		}

		// Other errors (e.g. timeout, or daemon internal error)
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
		_, _ = fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("CLI"),
			term.BoldString("v%d", local.ProtocolVersion),
		)
		return nil
	}

	// 4. Print daemon version
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("%s", term.BoldString("Lynx Daemon")))
	printVersionInfo(w, *daemonInfo)

	// 5. Print Protocol
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("CLI"),
		term.BoldString("v%d", local.ProtocolVersion),
	)
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("Daemon"),
		term.BoldString("v%d", daemonInfo.ProtocolVersion),
	)

	return nil
}

func handleProtocolMismatch(w io.Writer, local version.Info, err error) bool {
	var remoteErr *protocol.RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.Code != "PROTOCOL_MISMATCH" {
		return false
	}

	// Extract supported version safely using typed struct
	var supported int
	if data, ok := remoteErr.Data.(protocol.MismatchData); ok {
		supported = data.Supported
	}

	// Print Protocol section with error details
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.CyanString("Protocol"))
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("CLI"),
		term.BoldString("v%d", local.ProtocolVersion),
	)
	if supported > 0 {
		_, _ = fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("Daemon"),
			term.BoldString("v%d", supported),
		)
	} else {
		_, _ = fmt.Fprintf(
			w,
			"  %s : %s\n",
			term.DimString("Daemon"),
			term.BoldString("unknown"),
		)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%s\n", term.RedString("Error: Protocol mismatch"))
	if supported > 0 {
		_, _ = fmt.Fprintf(
			w,
			"The CLI (v%d) and Daemon (v%d) have incompatible protocols.\n",
			local.ProtocolVersion,
			supported,
		)
	} else {
		_, _ = fmt.Fprintf(
			w,
			"The CLI (v%d) and Daemon have incompatible protocols.\n",
			local.ProtocolVersion,
		)
	}

	// Return true to indicate handled
	return true
}

func printVersionInfo(w io.Writer, info version.Info) {
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("Version"),
		term.BoldString("%s", info.Version),
	)
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("Commit"),
		term.BoldString("%s", info.Commit),
	)
	_, _ = fmt.Fprintf(
		w,
		"  %s : %s\n",
		term.DimString("Built"),
		term.BoldString("%s", info.BuildDate),
	)
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "version",
		Usage:       term.BoldString("lynx version"),
		Description: "Show version information for CLI and Daemon.",
		Options: []help.Option{
			{Long: "--json", Description: "Output version info as JSON."},
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message for the version command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
