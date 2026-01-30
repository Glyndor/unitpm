package restart

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the restart command.
func Run(client *transport.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing process ID or name")
	}

	for _, id := range args {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		
		err := client.Call("restart", map[string]string{"id": id}, &resp)
		if err != nil {
			term.Printf("Failed to restart %s: %v\n", id, err)
			continue
		}
		term.Printf("Restarted %s\n", resp.ID)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "restart",
		Description: "Restart a process",
		Usage:       "lynx restart <id|name>...",
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
