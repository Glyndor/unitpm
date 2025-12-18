// Package root implements the root command.
package root

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
)

const (
	cmdList    = "list"
	cmdVersion = "version"
)

// Execute executes the root CLI command.
func Execute(args []string) error {
	if len(args) < 1 {
		printError(os.Stderr, "Missing command")
		printHelp(os.Stderr)
		os.Exit(1)
		return nil
	}

	command := normalizeCommand(args[0])

	// Handle global help
	if command == "-h" || command == "--help" {
		printHelp(os.Stdout)
		return nil
	}

	// Handle subcommand help (bypass IPC)
	if len(args) > 1 && isHelpRequest(args[1:]) {
		switch command {
		case cmdList:
			list.PrintHelp()
			return nil
		case cmdVersion:
			version.PrintHelp()
			return nil
		}
	}

	var cmdErr error

	switch command {
	case cmdList:
		client, err := ipc.NewClient()
		if err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer func() {
			_ = client.Close()
		}()
		cmdErr = list.Run(client, args[1:])

	case cmdVersion:
		cmdErr = version.Run(os.Stdout, args[1:])

	default:
		// Unknown command or flag
		printError(os.Stderr, "Command not found: %s", args[0])
		printHelp(os.Stderr)
		os.Exit(1)
		return nil
	}

	if cmdErr != nil {
		var usageErr *errs.UsageError
		if errors.As(cmdErr, &usageErr) {
			printError(os.Stderr, "%s", usageErr.Message)
			switch command {
			case cmdList:
				list.PrintHelp()
			case cmdVersion:
				version.PrintHelp()
			}
			os.Exit(1)
		}
		return cmdErr
	}

	return nil
}

func normalizeCommand(cmd string) string {
	switch cmd {
	case "ls", "ps":
		return cmdList
	case "--version":
		return cmdVersion
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

func printError(w io.Writer, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(w, "%s\n", term.RedString("[Lynx][ERROR] %s", msg))
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "\n%s\n", term.CyanString("Usage:"))
	fmt.Fprintf(w, "  lynx <command> [flags]\n")
	fmt.Fprintf(w, "\n%s\n", term.CyanString("Commands:"))
	fmt.Fprintf(w, "  %s\tList managed processes\n", term.BoldString("list, ls, ps"))
	fmt.Fprintf(w, "  %s\tShow version information\n", term.BoldString("version"))
	fmt.Fprintf(w, "\n%s\n", term.CyanString("Get Help:"))
	fmt.Fprintf(w, "  lynx --help\n")
	fmt.Fprintf(w, "  lynx <command> --help\n")
}
