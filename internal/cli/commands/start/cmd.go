// Package start implements the start command.
package start

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	name          string
	cwd           string
	stdio         string
	runAs         string
	cmdParts      []string
	cron          string
	runtime       string
	envFile       string
	shell         bool
	restartPolicy string
	maxRestarts   int
	restartDelay  int
	backoff       string
	stopOnExit    []int
	logDir        string
	stdout        string
	stderr        string
	logFormat     string
	logTimestamp  string

	parsingFlags bool
}

func (p *specParser) parse() (protocol.AppSpec, error) {
	p.parsingFlags = true
	p.stdio = "inherit"
	p.runAs = "self"
	p.restartPolicy = "on-failure"
	p.maxRestarts = 10
	p.restartDelay = 2000
	p.backoff = "expo"
	p.stopOnExit = []int{0}
	p.logFormat = "plain"
	p.logTimestamp = "rfc3339"

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
	case "--cron", "--schedule":
		return p.readStringValue(&p.cron)
	case "--runtime":
		return p.readStringValue(&p.runtime)
	case "--shell":
		p.shell = true
		return nil
	case "--env-file":
		return p.readStringValue(&p.envFile)
	case "--restart":
		return p.readStringValue(&p.restartPolicy)
	case "--max-restarts":
		return p.readIntValue(&p.maxRestarts)
	case "--restart-delay":
		return p.readIntValue(&p.restartDelay)
	case "--backoff":
		return p.readStringValue(&p.backoff)
	case "--stop-on-exit":
		return p.readIntList(&p.stopOnExit)
	case "--log-dir":
		return p.readStringValue(&p.logDir)
	case "--stdout":
		return p.readStringValue(&p.stdout)
	case "--stderr":
		return p.readStringValue(&p.stderr)
	case "--log-format":
		return p.readStringValue(&p.logFormat)
	case "--log-timestamp":
		return p.readStringValue(&p.logTimestamp)
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

func (p *specParser) readIntValue(target *int) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	val, err := strconv.Atoi(p.args[p.pos])
	if err != nil {
		return fmt.Errorf("invalid integer value: %s", p.args[p.pos])
	}
	*target = val
	return nil
}

func (p *specParser) readIntList(target *[]int) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	parts := strings.Split(p.args[p.pos], ",")
	var list []int
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		val, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid integer in list: %s", s)
		}
		list = append(list, val)
	}
	*target = list
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
			Mode:      p.stdio,
			Stdout:    p.stdout,
			Stderr:    p.stderr,
			Dir:       p.logDir,
			Format:    p.logFormat,
			Timestamp: p.logTimestamp,
		},
		Restart: &protocol.AppRestart{
			Policy:      p.restartPolicy,
			MaxRetries:  p.maxRestarts,
			BackoffMs:   p.restartDelay,
			BackoffType: p.backoff,
			StopOnExit:  p.stopOnExit,
		},
		RunAs: &protocol.RunAsPolicy{
			Mode: p.runAs,
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
				Shell:   p.shell,
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
					Shell:   p.shell,
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
						Shell:   p.shell,
					}
				case ".go":
					spec.Exec = protocol.AppExec{
						Type:    "entry",
						Entry:   token,
						Runtime: "go run",
						Shell:   p.shell,
					}
				default:
					// Treat as simple command
					spec.Exec = protocol.AppExec{
						Type:    "command",
						Command: token,
						Shell:   p.shell,
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
			Shell:   p.shell,
		}
	}

	return spec, nil
}

func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
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
			{Short: "", Long: "--name <name>", Description: "Assign a name to the process"},
			{Short: "", Long: "--cwd <dir>", Description: "Working directory"},
			{Short: "", Long: "--shell", Description: "Execute command in shell"},
			{Short: "", Long: "--schedule <cron>", Description: "Cron schedule for restart (alias --cron)"},
			{Short: "", Long: "--restart <policy>", Description: "Restart policy (never, on-failure, always)"},
			{Short: "", Long: "--max-restarts <N>", Description: "Max restarts (default 10)"},
			{Short: "", Long: "--restart-delay <ms>", Description: "Restart delay in ms (default 2000)"},
			{Short: "", Long: "--backoff <type>", Description: "Backoff strategy (none, linear, expo)"},
			{Short: "", Long: "--stop-on-exit <codes>", Description: "Exit codes to stop on (comma-separated)"},
			{Short: "", Long: "--log-dir <path>", Description: "Directory for logs"},
			{Short: "", Long: "--stdout <file>", Description: "Stdout file (relative to log-dir)"},
			{Short: "", Long: "--stderr <file>", Description: "Stderr file (relative to log-dir)"},
			{Short: "", Long: "--log-format <fmt>", Description: "Log format (plain, json)"},
			{Short: "", Long: "--log-timestamp <fmt>", Description: "Log timestamp (rfc3339, unix, none)"},
			{Short: "", Long: "--runtime <rt>", Description: "Runtime for entry file (e.g., node, python)"},
			{Short: "", Long: "--env-file <file>", Description: "Path to environment file"},
		},
	}
}
