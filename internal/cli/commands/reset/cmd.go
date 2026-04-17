// Package reset implements the reset command: zeroes a process's Restarts
// counter without stopping or restarting the process itself.
package reset

import (
	"errors"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the reset command. Client is created lazily after argument
// validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if len(args) == 0 {
		return errors.New("missing process ID or name")
	}

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	for _, id := range args {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		if err := client.Call("reset", map[string]string{"id": id}, &resp); err != nil {
			_, _ = term.Printf("%s Failed to reset %s: %v\n", term.RedString("✗"), id, err)
			continue
		}
		_, _ = term.Printf("%s Reset %s\n", term.GreenString("✓"), resp.ID)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "reset",
		Description: "Reset a process's Restarts counter to zero",
		Usage:       "lynx reset <id|name>...",
		Examples: []string{
			`lynx reset api`,
			`lynx reset prod:worker`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
