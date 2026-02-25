//go:build linux

package reload

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the reload command to restart a specific application.
func Run(client *transport.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing process ID or name")
	}

	for _, id := range args {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}

		err := client.Call("reload", map[string]string{"id": id}, &resp)
		if err != nil {
			term.Printf("Failed to reload %s: %v\n", id, err)
			continue
		}
		term.Printf("Reloaded %s\n", resp.ID)
	}
	return nil
}

// GetSpec returns the command specification for the reload command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "reload",
		Usage:       "lynx reload <id|name>...",
		Description: "Reload process configuration and restart",
	}
}

// PrintHelp prints the help information for the reload command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
