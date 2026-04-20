// Package flush implements the flush command: truncates a process's log files.
package flush

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
	"github.com/Jaro-c/Lynx/internal/cli/format"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the flush command to clear logs for a specific application.
// Client is created lazily after argument validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("flush", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Flush every process in this namespace")

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

	rep := batch.New("flush")
	for _, id := range ids {
		var resp struct {
			Status     string `json:"status"`
			ID         string `json:"id"`
			BytesFreed int64  `json:"bytes_freed,omitempty"`
		}
		if err := client.Call("flush", map[string]string{"id": id}, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to flush %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		if !jsonOut {
			if resp.BytesFreed > 0 {
				_, _ = term.Printf("%s Flushed logs for %s %s\n",
					term.GreenString("✓"), resp.ID,
					term.DimString("(%s freed)", format.Bytes(resp.BytesFreed)),
				)
			} else {
				_, _ = term.Printf("%s Flushed logs for %s\n", term.GreenString("✓"), resp.ID)
			}
		}
		extra := map[string]any{}
		if resp.BytesFreed > 0 {
			extra["bytes_freed"] = resp.BytesFreed
		}
		rep.OK(resp.ID, extra)
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

// GetSpec returns the command specification for the flush command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "flush",
		Usage:       "lynxpm flush <id|name|ns:*|*>... [--namespace <ns>] [--json]",
		Description: "Flush logs for a process",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--namespace <ns>", Description: "Flush every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			"lynxpm flush api",
			"lynxpm flush prod:api prod:worker",
			"lynxpm flush 'prod:*'        # every process in namespace prod (quote the glob)",
			"lynxpm flush --namespace prod # equivalent, no shell quoting needed",
			"lynxpm flush api --json | jq '.results[].extra.bytes_freed'",
		},
	}
}

// PrintHelp prints the help information for the flush command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
