// Package apply implements the apply command: applies a Lynxfile.yml declarative configuration to the daemon.
package apply

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/batch"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/lynxfile"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the apply command to load a Lynxfile and start the defined applications.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "Emit a machine-readable batch report")

	flagArgs, rest := batch.SplitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			return &errs.UsageError{Message: "Unknown flag: -" + strings.TrimPrefix(err.Error(), "flag provided but not defined: -")}
		}
		return &errs.UsageError{Message: err.Error()}
	}

	if len(rest) == 0 {
		return errs.NewUsageError("missing Lynxfile path")
	}
	path := rest[0]

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open Lynxfile: %w", err)
	}
	defer func() { _ = f.Close() }()

	file, err := lynxfile.Parse(f)
	if err != nil {
		return err
	}

	specs, err := file.ToAppSpecs()
	if err != nil {
		return err
	}

	// Connect only after local validation succeeds.
	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	rep := batch.New("apply")
	for _, s := range specs {
		id, err := spec.GenerateID()
		if err != nil {
			rep.Fail(s.Name, err)
			return emitAndReturn(rep, jsonOut, fmt.Errorf("failed to generate ID: %w", err))
		}

		s.ID = id
		if s.Namespace == "" {
			s.Namespace = "default"
		}
		s.CreatedAt = time.Now().Format(time.RFC3339)

		if s.Env == nil {
			s.Env = make(map[string]string)
		}

		if _, err := spec.SaveSpec(s.ID, s); err != nil {
			rep.Fail(fmt.Sprintf("%s/%s", s.Namespace, s.Name), err)
			return emitAndReturn(rep, jsonOut, fmt.Errorf("failed to save spec: %w", err))
		}

		req := protocol.StartRequest{
			ProtocolVersion: 1,
			Type:            "start",
			RequestID:       s.ID,
			Spec:            s,
		}

		var resp protocol.StartResponseData
		target := fmt.Sprintf("%s/%s", s.Namespace, s.Name)
		if err := client.Call("start", req, &resp); err != nil {
			rep.Fail(target, err)
			return emitAndReturn(rep, jsonOut, fmt.Errorf("apply failed for %s: %w", s.Name, err))
		}

		rep.OK(target, map[string]any{"id": s.ID, "pid": resp.PID})
		if !jsonOut {
			_, _ = term.Printf("%s Applied %s\n", term.GreenString("✓"), target)
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

// emitAndReturn finalizes the report when a fatal (abort-on-error) apply
// step fails mid-loop. In JSON mode the partial report goes to stdout so
// callers can see exactly which targets were applied before the failure.
func emitAndReturn(rep *batch.Report, jsonOut bool, wrapErr error) error {
	if jsonOut {
		_ = rep.EmitJSON()
	}
	return wrapErr
}

// GetSpec returns the command specification for the apply command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "apply",
		Usage:       "lynxpm apply <Lynxfile.yml> [--json]",
		Description: "Apply a Lynxfile.yml declarative configuration",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--json", Description: "Emit a machine-readable batch report."},
		},
		Examples: []string{
			"lynxpm apply Lynxfile.yml",
			"lynxpm apply config/production.yml",
			"lynxpm apply Lynxfile.yml --json | jq '.results'",
		},
	}
}

// PrintHelp prints the help information for the apply command.
func PrintHelp(w ...interface{}) {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
