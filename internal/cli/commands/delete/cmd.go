// Package deletecmd implements the delete command: removes a process and its spec from the daemon.
package deletecmd

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

// Run executes the delete command.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		purge     bool
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&purge, "purge", false, "Delete logs and runtime data")
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Delete every process in this namespace")

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

	rep := batch.New("delete")
	for _, id := range ids {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		req := map[string]any{"id": id, "purge": purge}
		if err := client.Call("delete", req, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to delete %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		if !jsonOut {
			_, _ = term.Printf("%s Deleted %s\n", term.GreenString("✓"), resp.ID)
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
		Name:        "delete",
		Aliases:     []string{"remove", "rm"},
		Description: "Delete a process and its spec",
		Usage:       "lynxpm delete|remove|rm [--purge] [--namespace <ns>] [--json] <id|name|ns:*|*>...",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--purge", Description: "Delete logs and runtime data."},
			{Short: "", Long: "--namespace <ns>", Description: "Delete every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			`lynxpm delete api`,
			`lynxpm rm --purge old-worker`,
			`lynxpm delete prod:api prod:worker`,
			`lynxpm delete 'prod:*'        # every process in namespace prod (quote the glob)`,
			`lynxpm rm --namespace prod    # equivalent, no shell quoting needed`,
			`lynxpm rm api worker --json | jq '.summary'`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
