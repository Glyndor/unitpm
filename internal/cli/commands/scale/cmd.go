// Package scale implements the scale command: brings the number of
// instances of an app (name + namespace) to a target count.
package scale

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/batch"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the scale command. Expects two positional args: <name> <N>.
// Name may be namespace-qualified as "ns:name".
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("scale", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "Emit the scale result as JSON on stdout")

	flagArgs, rest := batch.SplitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			return &errs.UsageError{Message: "Unknown flag: -" + strings.TrimPrefix(err.Error(), "flag provided but not defined: -")}
		}
		return &errs.UsageError{Message: err.Error()}
	}

	if len(rest) < 2 {
		return errors.New("usage: lynxpm scale <name> <N>")
	}

	target := rest[1]
	namespace := ""
	name := rest[0]
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

	if jsonOut {
		b, err := jsonx.Marshal(resp)
		if err != nil {
			return fmt.Errorf("json encode failed: %w", err)
		}
		_, err = fmt.Fprintln(os.Stdout, string(b))
		return err
	}

	_, _ = term.Printf("%s Scaled %s: %d → %d\n", term.GreenString("✓"), name, resp.Before, resp.After)
	for _, c := range resp.Created {
		_, _ = term.Printf("  %s %s\n", term.GreenString("+"), c)
	}
	for _, d := range resp.Deleted {
		_, _ = term.Printf("  %s %s\n", term.RedString("-"), d)
	}
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "scale",
		Description: "Scale an app up or down to the target number of instances",
		Usage:       "lynxpm scale <name> <N> [--json]",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--json", Description: "Emit the scale result as JSON on stdout."},
		},
		Examples: []string{
			`lynxpm scale worker 5          # set 'worker' to exactly 5 instances`,
			`lynxpm scale prod:api 10       # namespace-qualified`,
			`lynxpm scale worker 0          # stop all instances (equivalent to delete all)`,
			`lynxpm scale worker 5 --json`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
