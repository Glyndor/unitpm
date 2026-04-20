// Package reset implements the reset command: zeroes a process's Restarts
// counter without stopping or restarting the process itself.
package reset

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/batch"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/expand"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the reset command. Client is created lazily after argument
// validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Reset every process in this namespace")

	flagArgs, ids := batch.SplitArgsWithValues(args, map[string]bool{expand.NamespaceFlag: true})
	if err := fs.Parse(flagArgs); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			return &errs.UsageError{
				Message: "Unknown flag: -" + strings.TrimPrefix(err.Error(), "flag provided but not defined: -"),
			}
		}
		return &errs.UsageError{Message: err.Error()}
	}
	if len(ids) == 0 && namespace == "" {
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

	ids, err := expand.Targets(client, ids, namespace)
	if err != nil {
		return err
	}

	rep := batch.New("reset")
	for _, id := range ids {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		if err := client.Call("reset", map[string]string{"id": id}, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to reset %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		if !jsonOut {
			_, _ = term.Printf("%s Reset %s\n", term.GreenString("✓"), resp.ID)
		}
		rep.OK(resp.ID, nil)
	}

	if jsonOut {
		if err := rep.EmitJSON(); err != nil {
			return fmt.Errorf("json emit failed: %w", err)
		}
		return rep.Err()
	}
	rep.PrintSummary()
	return rep.Err()
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "reset",
		Description: "Reset a process's Restarts counter to zero",
		Usage:       "lynxpm reset <id|name|ns:*|*>... [--namespace <ns>] [--json]",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--namespace <ns>", Description: "Reset every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			`lynxpm reset api`,
			`lynxpm reset prod:worker`,
			`lynxpm reset 'prod:*'        # every process in namespace prod (quote the glob)`,
			`lynxpm reset --namespace prod # equivalent, no shell quoting needed`,
			`lynxpm reset api worker --json | jq '.summary'`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
