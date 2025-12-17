// Package root implements the root command.
package root

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Execute executes the root CLI command.
func Execute() error {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lynx <command>")
		return nil
	}

	command := normalizeCommand(os.Args[1])

	// Common client setup
	client, err := ipc.NewClient()
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	switch command {
	case "ping":
		return runPing(client)
	case "list":
		return list.Run(client)
	case "start", "stop":
		// Placeholder for start/stop commands as requested in the prompt
		// These would be implemented in their respective packages
		return fmt.Errorf("command '%s' not fully implemented in refactor yet", command)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func normalizeCommand(cmd string) string {
	switch cmd {
	case "ls", "ps", "status":
		return "list"
	default:
		return cmd
	}
}

func runPing(client *ipc.Client) error {
	var result map[string]string
	if err := client.Call("ping", nil, &result); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))
	return nil
}
