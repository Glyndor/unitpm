package show

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

type showResponse struct {
	Info struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Namespace string  `json:"namespace"`
		Version   string  `json:"version"`
		Mode      string  `json:"mode"`
		PID       int     `json:"pid"`
		Uptime    int64   `json:"uptime_ms"`
		Restarts  int     `json:"restarts"`
		State     string  `json:"state"`
		CPU       float64 `json:"cpu"`
		Memory    int64   `json:"memory_bytes"`
		User      string  `json:"user"`
		Watch     bool    `json:"watch"`
		CreatedAt string  `json:"created_at"`
	} `json:"info"`
	Spec any `json:"spec"`
}

func Run(client *transport.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing process ID or name")
	}

	id := args[0]

	var resp showResponse
	if err := client.Call("show", map[string]string{"id": id}, &resp); err != nil {
		return fmt.Errorf("show failed: %w", err)
	}

	term.Printf("Process %s (%s)\n", resp.Info.Name, resp.Info.ID)
	term.Printf("Namespace: %s\n", resp.Info.Namespace)
	term.Printf("State: %s\n", resp.Info.State)
	term.Printf("PID: %d\n", resp.Info.PID)
	term.Printf("CPU: %.1f%%  Memory: %d bytes\n", resp.Info.CPU, resp.Info.Memory)
	term.Printf("Uptime: %d ms  Restarts: %d\n", resp.Info.Uptime, resp.Info.Restarts)
	term.Printf("User: %s  Mode: %s  Version: %s\n", resp.Info.User, resp.Info.Mode, resp.Info.Version)

	return nil
}

func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "show",
		Usage:       "lynx show <id|name|namespace:name>",
		Description: "Show detailed information about a process",
	}
}

func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
