package reload

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

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

func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "reload",
		Usage:       "lynx reload <id|name>...",
		Description: "Reload process configuration and restart",
	}
}

func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
