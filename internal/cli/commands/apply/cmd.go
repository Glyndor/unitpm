package apply

import (
	"fmt"
	"os"
	"time"

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

	if len(args) == 0 {
		return errs.NewUsageError("missing Lynxfile path")
	}

	path := args[0]

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open Lynxfile: %w", err)
	}
	defer f.Close()

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

	for _, s := range specs {
		id, err := spec.GenerateID()
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
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
			return fmt.Errorf("failed to save spec: %w", err)
		}

		req := protocol.StartRequest{
			ProtocolVersion: 1,
			Type:            "start",
			RequestID:       s.ID,
			Spec:            s,
		}

		var resp protocol.StartResponseData
		if err := client.Call("start", req, &resp); err != nil {
			return fmt.Errorf("apply failed for %s: %w", s.Name, err)
		}

		term.Printf("Applied %s/%s\n", s.Namespace, s.Name)
	}

	return nil
}

// GetSpec returns the command specification for the apply command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "apply",
		Usage:       "lynx apply <Lynxfile.yml>",
		Description: "Apply a Lynxfile.yml declarative configuration",
	}
}

// PrintHelp prints the help information for the apply command.
func PrintHelp(w ...interface{}) {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
