package flush

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the flush command to clear logs for a specific application.
func Run(client transport.IPCClient, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing process ID or name")
	}

	for _, id := range args {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}

		err := client.Call("flush", map[string]string{"id": id}, &resp)
		if err != nil {
			term.Printf("Failed to flush %s: %v\n", id, err)
			continue
		}
		term.Printf("Flushed logs for %s\n", resp.ID)
	}
	return nil
}

// GetSpec returns the command specification for the flush command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "flush",
		Usage:       "lynx flush <id|name>...",
		Description: "Flush logs for a process",
	}
}

// PrintHelp prints the help information for the flush command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
