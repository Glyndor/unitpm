// Package deletecmd implements the delete command: removes a process and its spec from the daemon.
package deletecmd

import (
	"errors"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the delete command.
func Run(client transport.IPCClient, args []string) error {
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
			_, _ = term.Printf("%s Failed to delete %s: %v\n", term.RedString("✗"), id, err)
			continue
		}
		_, _ = term.Printf("%s Deleted %s\n", term.GreenString("✓"), resp.ID)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "delete",
		Aliases:     []string{"remove", "rm"},
		Description: "Delete a process and its spec",
		Usage:       "lynxpm delete [--purge] <id|name>...",
		Options: []help.Option{
			{Short: "", Long: "--purge", Description: "Delete logs and runtime data"},
		},
		Examples: []string{
			`lynxpm delete api`,
			`lynxpm rm --purge old-worker`,
			`lynxpm delete prod:api prod:worker`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
