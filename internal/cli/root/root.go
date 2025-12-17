// Package root implements the root command.
package root

import (
	"fmt"
	"io"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
)

const (
	cmdList  = "list"
	cmdStart = "start"
	cmdStop  = "stop"
	cmdPing  = "ping"
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

	// Validate command before connecting
	switch command {
	case cmdPing, cmdList, cmdStart, cmdStop:
		// Valid command, proceed
	default:
		fmt.Fprintf(os.Stderr,
			"%s\n",
			term.RedString("[Lynx][ERROR] Command not found: %s", os.Args[1]))
		printHelp(os.Stderr)
		os.Exit(1)
		return nil
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
	case cmdPing:
		return runPing(client)
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
	case "ls", "ps", "status":
		return cmdList
	default:
		return cmd
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "\n%s\n", term.CyanString("Usage:"))
	fmt.Fprintf(w, "  lynx <command> [flags]\n")

	fmt.Fprintf(w, "\n%s\n", term.CyanString("Get Help:"))
	fmt.Fprintf(w, "  lynx --help\n")
	fmt.Fprintf(w, "  lynx <command> --help\n")
}

func runPing(client *ipc.Client) error {
	var result map[string]string
	if err := client.Call("ping", nil, &result); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))
	return nil
}
