package stop

import (
	"errors"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the stop command.
func Run(client transport.IPCClient, args []string) error {
	if len(args) == 0 {
		return errors.New("missing process ID or name")
	}

	for _, id := range args {
		var resp struct {
			Status     string `json:"status"`
			ID         string `json:"id"`
			WasRunning bool   `json:"was_running"`
		}

		err := client.Call("stop", map[string]string{"id": id}, &resp)
		if err != nil {
			term.Printf("Failed to stop %s: %v\n", id, err)
			continue
		}
		if resp.WasRunning {
			term.Printf("Stopped %s\n", resp.ID)
		} else {
			term.Printf("Already stopped: %s\n", resp.ID)
		}
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "stop",
		Description: "Stop a running process",
		Usage:       "lynx stop <id|name>...",
		Examples: []string{
			`lynx stop api`,
			`lynx stop prod:api`,
			`lynx stop api worker-1 worker-2`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
