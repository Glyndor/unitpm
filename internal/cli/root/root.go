// Package root implements the root command.
package root

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/cli/commands/start"
	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/registry"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

const (
	cmdList    = "list"
	cmdStart   = "start"
	cmdVersion = "version"
)

// Execute executes the root CLI command.
func Execute(args []string) int {
	registerCommands()
	specs := registry.GetAll()

	if len(args) < 1 {
		help.RenderRootHelp(os.Stdout, specs, false)
		return 0
	}

	command, found := registry.Resolve(args[0])
	if !found {
		if args[0] == "--version" {
			command = cmdVersion
		} else {
			command = args[0]
		}
	}

	// Handle global help
	if command == "-h" || command == "--help" {
		help.RenderRootHelp(os.Stdout, specs, true)
		return 0
	}

	// Handle subcommand help (bypass IPC)
	if len(args) > 1 && isHelpRequest(args[1:]) {
		switch command {
		case cmdList:
			list.PrintHelp()
			return 0
		case cmdStart:
			start.PrintHelp()
			return 0
		case cmdVersion:
			version.PrintHelp()
			return 0
		}
	}

	var cmdErr error

	switch command {
	case cmdList:
		client, err := transport.NewClient()
		if err != nil {
			printError(os.Stderr, "failed to connect to daemon: %v", err)
			return 1
		}
		defer func() {
			_ = client.Close()
		}()
		cmdErr = list.Run(client, args[1:])

	case cmdStart:
		client, err := transport.NewClient()
		if err != nil {
			printError(os.Stderr, "failed to connect to daemon: %v", err)
			return 1
		}
		defer func() {
			_ = client.Close()
		}()
		cmdErr = start.Run(client, args[1:])

	case cmdVersion:
		cmdErr = version.Run(os.Stdout, args[1:])

	default:
		// Unknown command or flag
		printError(os.Stderr, "Command not found: %s", args[0])
		help.RenderRootHelp(os.Stderr, specs, false)
		return 1
	}

	if cmdErr != nil {
		var usageErr *errs.UsageError
		if errors.As(cmdErr, &usageErr) {
			printError(os.Stderr, "%s", usageErr.Message)
			switch command {
			case cmdList:
				list.PrintHelp()
			case cmdStart:
				start.PrintHelp()
			case cmdVersion:
				version.PrintHelp()
			}
			return 1
		}
		printError(os.Stderr, "%v", cmdErr)
		return 1
	}

	return 0
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

func registerCommands() {
	registry.Register(list.GetSpec())
	registry.Register(start.GetSpec())
	registry.Register(version.GetSpec())
}
