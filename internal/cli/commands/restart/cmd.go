// Package restart implements the restart command: stops and restarts a process via the daemon.
package restart

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

// Run executes the restart command. Client is created lazily after
// argument validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("restart", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Restart every process in this namespace")

	flagArgs, ids := batch.SplitArgsWithValues(args, map[string]bool{expand.NamespaceFlag: true})
	if err := fs.Parse(flagArgs); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			return &errs.UsageError{Message: "Unknown flag: -" + strings.TrimPrefix(err.Error(), "flag provided but not defined: -")}
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

	rep := batch.New("restart")
	for _, id := range ids {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		if err := client.Call("restart", map[string]string{"id": id}, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to restart %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		if !jsonOut {
			_, _ = term.Printf("%s Restarted %s\n", term.GreenString("✓"), resp.ID)
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
		Name:        "restart",
		Description: "Restart a process",
		Usage:       "lynxpm restart <id|name|ns:*|*>... [--namespace <ns>] [--json]",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--namespace <ns>", Description: "Restart every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			"lynxpm restart api",
			"lynxpm restart prod:api worker",
			"lynxpm restart 'prod:*'        # every process in namespace prod (quote the glob)",
			"lynxpm restart --namespace prod # equivalent, no shell quoting needed",
			"lynxpm restart api --json",
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
