// Package flush implements the flush command: truncates a process's log files.
package flush

import (
	"errors"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the flush command to clear logs for a specific application.
// Client is created lazily after argument validation if nil.
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

		err := client.Call("flush", map[string]string{"id": id}, &resp)
		if err != nil {
			_, _ = term.Printf("%s Failed to flush %s: %v\n", term.RedString("✗"), id, err)
			continue
		}
		_, _ = term.Printf("%s Flushed logs for %s\n", term.GreenString("✓"), resp.ID)
	}
	return nil
}

// GetSpec returns the command specification for the flush command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "flush",
		Usage:       "lynxpm flush <id|name>...",
		Description: "Flush logs for a process",
		Examples: []string{
			"lynxpm flush api",
			"lynxpm flush prod:api prod:worker",
		},
	}
}

// PrintHelp prints the help information for the flush command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
