// Package root implements the root command.
package root

import (
	"fmt"
	"io"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
)

const (
	cmdList    = "list"
	cmdStart   = "start"
	cmdStop    = "stop"
	cmdVersion = "version"
)

// Execute executes the root CLI command.
func Execute() error {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr,
			"%s\n",
			term.RedString("[Lynx][ERROR] Missing command"))
		printHelp(os.Stderr)
		os.Exit(1)
		return nil
	}

	command := normalizeCommand(os.Args[1])

	// Handle unsupported flag -V explicitly
	if command == "-V" {
		fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Unknown flag: -V (use --version)"))
		os.Exit(1)
	}

	// Handle global help
	if command == "-h" || command == "--help" {
		printHelp(os.Stdout)
		return nil
	}

	// Validate command before connecting
	switch command {
	case cmdList, cmdStart, cmdStop:
		// Valid command, proceed
	case cmdVersion:
		return version.Run(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr,
			"%s\n",
			term.RedString("[Lynx][ERROR] Command not found: %s", os.Args[1]))
		printHelp(os.Stderr)
		os.Exit(1)
		return nil
	}

	// Handle subcommand help (bypass IPC)
	// Check for help flags before connecting to daemon.
	// Future commands (start, stop, logs) must follow this pattern.
	if isHelpRequest(os.Args[2:]) {
		switch command {
		case cmdList:
			list.PrintHelp()
			return nil
		default:
			// No specific help for other commands yet
		}
	}

	// Common client setup
	client, err := ipc.NewClient()
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	switch command {
	case cmdList:
		return list.Run(client)
	case cmdStart, cmdStop:
		// Placeholder for start/stop commands as requested in the prompt
		// These would be implemented in their respective packages
		return fmt.Errorf("command '%s' not fully implemented in refactor yet", command)
	default:
		// Should be unreachable due to pre-validation
		return fmt.Errorf("unknown command: %s", command)
	}
}

func normalizeCommand(cmd string) string {
	switch cmd {
	case "ls", "ps":
		return cmdList
	case "--version":
		return cmdVersion
	case "-V":
		// Explicitly return it so it can be caught in Execute
		return "-V"
	default:
		return cmd
	}
}

func isHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "\n%s\n", term.CyanString("Usage:"))
	fmt.Fprintf(w, "  lynx <command> [flags]\n")

	fmt.Fprintf(w, "\n%s\n", term.CyanString("Get Help:"))
	fmt.Fprintf(w, "  lynx --help\n")
	fmt.Fprintf(w, "  lynx <command> --help\n")
}
