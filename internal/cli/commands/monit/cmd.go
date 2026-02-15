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

func Run(client *transport.Client, args []string) error {
	interval := time.Second * 2

	for {
		var processes []types.ProcessInfo
		if err := client.Call("list", nil, &processes); err != nil {
			return fmt.Errorf("monit failed: %w", err)
		}

		fmt.Print("\033[H\033[2J")
		term.Printf("Lynx monit\n")
		for _, p := range processes {
			term.Printf("%s/%s pid=%d state=%s cpu=%.1f%% mem=%d\n", p.Namespace, p.Name, p.PID, p.State, p.CPU, p.Memory)
		}

		time.Sleep(interval)
	}
}

func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "monit",
		Usage:       "lynx monit",
		Description: "Show live process statistics",
	}
}

func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
