// Package version implements the version command.
package version

import (
	"errors"
	"fmt"
	"io"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/version"
)

// Run executes the version command.
func Run(w io.Writer) error {
	local := version.Get()

	// 1. Print local CLI version
	fmt.Fprintf(w, "Lynx CLI\n")
	printVersionInfo(w, local)

	// 2. Attempt to connect to daemon
	// We handle the connection manually here because failure is not an error for this command.
	client, err := ipc.NewClient()
	if err != nil {
		// Daemon not running or unreachable.
		// Print only Protocol section and exit 0.
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Protocol\n")
		fmt.Fprintf(w, "  CLI     : v%d\n", local.ProtocolVersion)
		return nil
	}
	defer client.Close()

	// 3. Fetch daemon version
	var daemonInfo version.Info
	err = client.Call("version", nil, &daemonInfo)
	if err != nil {
		// Check for protocol mismatch error
		var remoteErr *ipc.RemoteError
		if errors.As(err, &remoteErr) && remoteErr.Code == "PROTOCOL_MISMATCH" {
			// Extract supported version safely using typed struct
			var supported int
			if data, ok := remoteErr.Data.(ipc.ProtocolMismatchData); ok {
				supported = data.Supported
			}

			// Print Protocol section with error details
			fmt.Fprintln(w)
			fmt.Fprintf(w, "Protocol\n")
			fmt.Fprintf(w, "  CLI     : v%d\n", local.ProtocolVersion)
			if supported > 0 {
				fmt.Fprintf(w, "  Daemon  : v%d\n", supported)
			} else {
				fmt.Fprintf(w, "  Daemon  : unknown\n")
			}

			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s\n", term.RedString("Error: Protocol mismatch"))
			if supported > 0 {
				fmt.Fprintf(w, "The CLI (v%d) and Daemon (v%d) have incompatible protocols.\n", local.ProtocolVersion, supported)
			} else {
				fmt.Fprintf(w, "The CLI (v%d) and Daemon have incompatible protocols.\n", local.ProtocolVersion)
			}
			fmt.Fprintf(w, "Please ensure both Lynx CLI and Daemon are updated.\n")
			return fmt.Errorf("protocol mismatch")
		}

		// Other errors (e.g. timeout, or daemon internal error)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Protocol\n")
		fmt.Fprintf(w, "  CLI     : v%d\n", local.ProtocolVersion)
		return nil
	}

	// 4. Print daemon version
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Lynx Daemon\n")
	printVersionInfo(w, daemonInfo)

	// 5. Print Protocol
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Protocol\n")
	fmt.Fprintf(w, "  CLI     : v%d\n", local.ProtocolVersion)
	fmt.Fprintf(w, "  Daemon  : v%d\n", daemonInfo.ProtocolVersion)

	return nil
}

func printVersionInfo(w io.Writer, v version.Info) {
	fmt.Fprintf(w, "  Version    : %s\n", v.Version)
	fmt.Fprintf(w, "  Commit     : %s\n", v.Commit)
	fmt.Fprintf(w, "  Build date : %s\n", v.BuildDate)
}
