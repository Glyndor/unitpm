package show

import (
	"errors"
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

// Run executes the show command to display detailed information about a
// specific application. Client is created lazily after argument validation
// if nil.
func Run(client transport.IPCClient, args []string) error {
	if len(args) == 0 {
		return errors.New("missing process ID or name")
	}

	id := args[0]

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

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
	term.Printf("User: %s  Mode: %s  Version: %s\n",
		resp.Info.User, resp.Info.Mode, resp.Info.Version)

	return nil
}

// GetSpec returns the command specification for the show command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "show",
		Usage:       "lynx show <id|name|namespace:name>",
		Description: "Show detailed information about a process",
	}
}

// PrintHelp prints the help information for the show command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
