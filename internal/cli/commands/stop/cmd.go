// Package stop implements the stop command: sends SIGTERM (then SIGKILL if needed) to a managed process.
package stop

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

// Run executes the stop command. Client is created lazily after
// argument validation if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		jsonOut   bool
		namespace string
	)
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")
	fs.StringVar(&namespace, expand.NamespaceFlag, "", "Stop every process in this namespace")

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

	rep := batch.New("stop")
	for _, id := range ids {
		var resp struct {
			Status     string `json:"status"`
			ID         string `json:"id"`
			WasRunning bool   `json:"was_running"`
		}
		if err := client.Call("stop", map[string]string{"id": id}, &resp); err != nil {
			if !jsonOut {
				_, _ = term.Printf("%s Failed to stop %s: %v\n", term.RedString("✗"), id, err)
			}
			rep.Fail(id, err)
			continue
		}
		extra := map[string]any{"was_running": resp.WasRunning}
		if resp.WasRunning {
			if !jsonOut {
				_, _ = term.Printf("%s Stopped %s\n", term.GreenString("✓"), resp.ID)
			}
			rep.OK(resp.ID, extra)
		} else {
			if !jsonOut {
				_, _ = term.Printf("%s Already stopped: %s\n", term.YellowString("!"), resp.ID)
			}
			rep.Noop(resp.ID, extra)
		}
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
		Name:        "stop",
		Description: "Stop a running process",
		Usage:       "lynxpm stop <id|name|ns:*|*>... [--namespace <ns>] [--json]",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--namespace <ns>", Description: "Stop every process in this namespace."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			`lynxpm stop api`,
			`lynxpm stop prod:api`,
			`lynxpm stop api worker-1 worker-2`,
			`lynxpm stop 'prod:*'        # every process in namespace prod (quote the glob)`,
			`lynxpm stop --namespace prod # equivalent, no shell quoting needed`,
			`lynxpm stop api --json`,
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
