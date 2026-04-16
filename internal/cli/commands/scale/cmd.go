// Package scale implements the scale command: brings the number of
// instances of an app (name + namespace) to a target count.
package scale

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the scale command. Expects two positional args: <name> <N>.
// Name may be namespace-qualified as "ns:name".
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}
	if len(args) < 2 {
		return errors.New("usage: lynx scale <name> <N>")
	}

	target := args[1]
	namespace := ""
	name := args[0]
	if idx := strings.Index(name, ":"); idx != -1 {
		namespace = name[:idx]
		name = name[idx+1:]
	}
	n, err := strconv.Atoi(target)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid target count %q (must be a non-negative integer)", target)
	}

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	var resp protocol.ScaleResponse
	req := map[string]any{"name": name, "namespace": namespace, "target": n}
	if err := client.Call("scale", req, &resp); err != nil {
		return fmt.Errorf("scale failed: %w", err)
	}
	term.Printf("Scaled %s: %d → %d\n", name, resp.Before, resp.After)
	for _, c := range resp.Created {
		term.Printf("  + %s\n", c)
	}
	for _, d := range resp.Deleted {
		term.Printf("  - %s\n", d)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "scale",
		Description: "Scale an app up or down to the target number of instances",
		Usage:       "lynx scale <name> <N>",
		Examples: []string{
			`lynx scale worker 5          # set 'worker' to exactly 5 instances`,
			`lynx scale prod:api 10       # namespace-qualified`,
			`lynx scale worker 0          # stop all instances (equivalent to delete all)`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
