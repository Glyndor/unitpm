// Package start implements the start command.
package start

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the start command.
func Run(client *transport.Client, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	appSpec, err := ParseAppSpec(args)
	if err != nil {
		return err
	}

	// Generate ID
	id, err := spec.GenerateUUIDv4()
	if err != nil {
		return fmt.Errorf("failed to generate ID: %w", err)
	}
	appSpec.Id = id
	appSpec.CreatedAt = time.Now().Format(time.RFC3339)

	// Save Spec to disk
	savedPath, err := spec.SaveSpec(appSpec.Id, appSpec)
	if err != nil {
		return fmt.Errorf("failed to save spec: %w", err)
	}
	_, _ = term.Printf("Spec saved to %s\n", savedPath)

	// Send Request
	req := protocol.StartRequest{
		ProtocolVersion: 1,
		Type:            "start",
		RequestID:       id, // Use same ID for request correlation
		Spec:            appSpec,
	}

	var startResp protocol.StartResponseData
	err = client.Call("start", req, &startResp)
	if err != nil {
		return fmt.Errorf("start failed: %w", err)
	}

	printSuccessResponse(&startResp, appSpec.Name)

	return nil
}

// ParseAppSpec parses command-line arguments into an AppSpec.
func ParseAppSpec(args []string) (protocol.AppSpec, error) {
	return (&specParser{args: args}).parse()
}

type specParser struct {
	args []string
	pos  int

	name     string
	cwd      string
	stdio    string
	runAs    string
	cmdParts []string
	cron     string
	runtime  string
	envFile  string
	shell    bool

	parsingFlags bool
}

func (p *specParser) parse() (protocol.AppSpec, error) {
	p.parsingFlags = true
	p.stdio = "inherit"
	p.runAs = "self"

	for p.pos = 0; p.pos < len(p.args); p.pos++ {
		arg := p.args[p.pos]

		if !p.parsingFlags {
			p.cmdParts = append(p.cmdParts, arg)
			continue
		}

		if arg == "--" {
			p.pos++ // Skip "--"
			// Append remaining args to cmdParts
			for ; p.pos < len(p.args); p.pos++ {
				p.cmdParts = append(p.cmdParts, p.args[p.pos])
			}
			break
		}

		if strings.HasPrefix(arg, "-") {
			// Check if it's a known flag
			err := p.handleFlag(arg)
			if err == nil {
				continue
			}

			// If unknown flag:
			// If we already have a command, treat it as an argument to that command.
			if len(p.cmdParts) > 0 {
				p.cmdParts = append(p.cmdParts, arg)
				continue
			}

			// If no command yet, it's an invalid flag for lynx
			return protocol.AppSpec{}, err
		}

		p.cmdParts = append(p.cmdParts, arg)
	}

	return p.finalize()
}

func (p *specParser) handleFlag(arg string) error {
	switch arg {
	case "--name":
		return p.readStringValue(&p.name)
	case "--cwd":
		return p.readStringValue(&p.cwd)
	case "--cron":
		return p.readStringValue(&p.cron)
	case "--runtime":
		return p.readStringValue(&p.runtime)
	case "--shell":
		p.shell = true
		return nil
	case "--env-file":
		return p.readStringValue(&p.envFile)
	// TODO: Add support for other flags if needed
	default:
		return fmt.Errorf("unknown flag: %s", arg)
	}
}

func (p *specParser) readStringValue(target *string) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	*target = p.args[p.pos]
	return nil
}

func (p *specParser) finalize() (protocol.AppSpec, error) {
	if len(p.cmdParts) == 0 {
		return protocol.AppSpec{}, errs.NewUsageError("missing command or entry file")
	}

	// Resolve CWD
	cwd := p.cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return protocol.AppSpec{}, fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return protocol.AppSpec{}, fmt.Errorf("failed to resolve absolute path for cwd: %w", err)
	}

	spec := protocol.AppSpec{
		Version: 1,
		Name:    p.name,
		Cwd:     cwd,
		Cron:    p.cron,
		Logs: &protocol.AppLogs{
			Mode: p.stdio,
		},
		Env: make(map[string]string),
	}

	// Analyze command parts for Entry vs Command
	if len(p.cmdParts) == 1 {
		// Single token: might be "bun dev" (quoted) or "main.js"
		token := p.cmdParts[0]

		// If explicit runtime is provided, treat as Entry
		if p.runtime != "" {
			spec.Exec = protocol.AppExec{
				Type:    "entry",
				Entry:   token,
				Runtime: p.runtime,
			}
		} else {
			// Try to tokenize to see if it's a command string
			parts, err := Tokenize(token)
			if err == nil && len(parts) > 1 {
				// It was a quoted string with multiple parts -> Command
				spec.Exec = protocol.AppExec{
					Type:    "command",
					Command: parts[0],
					Args:    parts[1:],
				}
			} else {
				// Single part. Check extension for inference.
				ext := filepath.Ext(token)
				switch ext {
				case ".js", ".mjs", ".cjs":
					spec.Exec = protocol.AppExec{
						Type:    "entry",
						Entry:   token,
						Runtime: "node",
					}
				case ".go":
					spec.Exec = protocol.AppExec{
						Type:    "entry",
						Entry:   token,
						Runtime: "go run",
					}
				default:
					// Treat as simple command
					spec.Exec = protocol.AppExec{
						Type:    "command",
						Command: token,
					}
				}
			}
		}
	} else {
		// Multiple tokens: "node index.js" -> Command
		spec.Exec = protocol.AppExec{
			Type:    "command",
			Command: p.cmdParts[0],
			Args:    p.cmdParts[1:],
		}
	}

	return spec, nil
}

func PrintHelp() {
	fmt.Println("Usage: lynx start <command|file> [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  --name <name>      Assign a name to the process")
	fmt.Println("  --cwd <dir>        Working directory")
	fmt.Println("  --cron <schedule>  Cron schedule")
	fmt.Println("  --runtime <rt>     Runtime for entry file (e.g., node, python)")
}

func printSuccessResponse(data *protocol.StartResponseData, name string) {
	fmt.Printf("Started %s\n", name)
	if len(data.ProcID) > 8 {
		fmt.Printf("  ID: %s (short: %s)\n", data.ProcID, data.ProcID[:8])
	} else {
		fmt.Printf("  ID: %s\n", data.ProcID)
	}
	fmt.Printf("  PID: %d\n", data.PID)
	fmt.Printf("  Status: %s\n", data.Status)
}

func printErrorResponse(err *protocol.StartError) {
	fmt.Printf("Error: %s (%s)\n", err.Message, err.Code)
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "start",
		Usage:       term.BoldString("lynx start <command|file> [flags]"),
		Description: "Start a new process.",
		Options: []help.Option{
			{Short: "", Long: "--name", Description: "Assign a name to the process"},
			{Short: "", Long: "--cwd", Description: "Working directory"},
			{Short: "", Long: "--shell", Description: "Execute command in shell"},
			{Short: "", Long: "--cron", Description: "Cron schedule"},
			{Short: "", Long: "--runtime", Description: "Runtime for entry file (e.g., node, python)"},
			{Short: "", Long: "--env-file", Description: "Path to environment file"},
		},
	}
}
