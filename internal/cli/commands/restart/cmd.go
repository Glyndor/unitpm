// Package restart implements the restart command: stops and restarts a process via the daemon.
package restart

import (
	"errors"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the restart command. Client is created lazily after
// argument validation if nil.
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

		err := client.Call("restart", map[string]string{"id": id}, &resp)
		if err != nil {
			_, _ = term.Printf("%s Failed to restart %s: %v\n", term.RedString("✗"), id, err)
			continue
		}
		_, _ = term.Printf("%s Restarted %s\n", term.GreenString("✓"), resp.ID)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "restart",
		Description: "Restart a process",
		Usage:       "lynxpm restart <id|name>...",
		Examples: []string{
			"lynxpm restart api",
			"lynxpm restart prod:api worker",
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
