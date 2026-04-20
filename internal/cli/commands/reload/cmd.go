// Package reload implements the reload command: re-reads a process's spec and restarts it.
package reload

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

// Run executes the reload command. Client is created lazily after argument
// validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("reload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Reload every process in this namespace")

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

	rep := batch.New("reload")
	for _, id := range ids {
		var resp struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		if err := client.Call("reload", map[string]string{"id": id}, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to reload %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		if !jsonOut {
			_, _ = term.Printf("%s Reloaded %s\n", term.GreenString("✓"), resp.ID)
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

// GetSpec returns the command specification for the reload command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "reload",
		Usage:       "lynxpm reload <id|name|ns:*|*>... [--namespace <ns>] [--json]",
		Description: "Reload process configuration and restart",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--namespace <ns>", Description: "Reload every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			"lynxpm reload api",
			"lynxpm reload prod:api",
			"lynxpm reload 'prod:*'        # every process in namespace prod (quote the glob)",
			"lynxpm reload --namespace prod # equivalent, no shell quoting needed",
			"lynxpm reload api worker --json",
		},
	}
}

// PrintHelp prints the help information for the reload command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
