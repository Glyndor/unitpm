package delete

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the delete command.
func Run(client *transport.Client, args []string) error {
	purge := false
	ids := []string{}

	for _, arg := range args {
		if arg == "--purge" {
			purge = true
		} else {
			ids = append(ids, arg)
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("missing process ID or name")
	}

	for _, id := range ids {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}

		req := map[string]interface{}{
			"id":    id,
			"purge": purge,
		}

		err := client.Call("delete", req, &resp)
		if err != nil {
			term.Printf("Failed to delete %s: %v\n", id, err)
			continue
		}
		term.Printf("Deleted %s\n", resp.ID)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "delete",
		Aliases:     []string{"remove", "rm"},
		Description: "Delete a process and its spec",
		Usage:       "lynx delete [--purge] <id|name>...",
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
