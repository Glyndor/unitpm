// Package monit implements the monit command: prints a refreshing live view of all managed processes.
package monit

import (
	"fmt"
	"os"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Run executes the monit command to display live statistics for all running
// applications. Client is created lazily if nil.
func Run(client transport.IPCClient, args []string) error {
	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	interval := time.Second * 2

	for {
		var processes []types.ProcessInfo
		if err := client.Call("list", nil, &processes); err != nil {
			return fmt.Errorf("monit failed: %w", err)
		}

		fmt.Print("\033[H\033[2J")
		_, _ = term.Printf("Lynx monit\n")
		for _, p := range processes {
			_, _ = term.Printf(
				"%s/%s pid=%d state=%s cpu=%.1f%% mem=%d\n",
				p.Namespace,
				p.Name,
				p.PID,
				p.State,
				p.CPU,
				p.Memory,
			)
		}

		time.Sleep(interval)
	}
}

// GetSpec returns the command specification for the monit command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "monit",
		Usage:       "lynxpm monit",
		Description: "Show live process statistics",
	}
}

// PrintHelp prints the help information for the monit command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
