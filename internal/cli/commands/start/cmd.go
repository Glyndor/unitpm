// Package start implements the start command.
package start

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the start command.
func Run(client *transport.Client, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	spec, err := ParseStartSpec(args)
	if err != nil {
		return err
	}

	// Send Request
	var startResp protocol.StartResponse
	err = client.Call("start", spec, &startResp)
	if err != nil {
		return fmt.Errorf("start failed: %w", err)
	}

	// Handle Response
	if !startResp.Ok {
		if startResp.Error != nil {
			printErrorResponse(startResp.Error)
			return errors.New("process start failed")
		}
		return errors.New("process start failed with unknown error")
	}

	if startResp.Data != nil {
		printSuccessResponse(startResp.Data, spec.Name)
	}

	return nil
}

// ParseStartSpec parses command-line arguments into a StartSpec.
func ParseStartSpec(args []string) (protocol.StartSpec, error) {
	return (&specParser{args: args}).parse()
}

type specParser struct {
	args []string
	pos  int

	name     string
	cwd      string
	stdio    string
	runAs    string
	username string
	envs     []string
	cmdParts []string

	parsingFlags bool
}

func (p *specParser) parse() (protocol.StartSpec, error) {
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
			p.parsingFlags = false
			continue
		}

		if strings.HasPrefix(arg, "-") {
			if err := p.handleFlag(arg); err != nil {
				return protocol.StartSpec{}, err
			}
			continue
		}

		p.cmdParts = append(p.cmdParts, arg)
	}

	return p.finalize()
}

func (p *specParser) handleFlag(arg string) error {
	var flagName, flagValue string
	var hasValue bool

	if strings.Contains(arg, "=") {
		parts := strings.SplitN(arg, "=", 2)
		flagName = parts[0]
		flagValue = parts[1]
		hasValue = true
	} else {
		flagName = arg
		hasValue = false
	}

	flagName = strings.TrimLeft(flagName, "-")

	getValue := func() (string, error) {
		if hasValue {
			return flagValue, nil
		}
		if p.pos+1 >= len(p.args) {
			return "", fmt.Errorf("flag --%s requires a value", flagName)
		}
		p.pos++
		return p.args[p.pos], nil
	}

	switch flagName {
	case "name":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.name = val
	case "cwd":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.cwd = val
	case "stdio":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.stdio = val
	case "run-as":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.runAs = val
	case "username":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.username = val
	case "env":
		val, err := getValue()
		if err != nil {
			return &errs.UsageError{Message: err.Error()}
		}
		p.envs = append(p.envs, val)
	case "cron":
		return &errs.UsageError{Message: "ERR_UNSUPPORTED: cron scheduling is not implemented yet"}
	default:
		p.cmdParts = append(p.cmdParts, arg)
	}
	return nil
}

func (p *specParser) finalize() (protocol.StartSpec, error) {
	if len(p.cmdParts) == 0 {
		return protocol.StartSpec{}, &errs.UsageError{Message: "Command is required"}
	}

	var cmd string
	var procArgs []string

	// Command Resolution Logic
	if len(p.cmdParts) == 1 && strings.Contains(p.cmdParts[0], " ") {
		tokenized, err := Tokenize(p.cmdParts[0])
		if err != nil {
			return protocol.StartSpec{}, &errs.UsageError{
				Message: fmt.Sprintf("Failed to parse command line: %v", err),
			}
		}
		if len(tokenized) == 0 {
			return protocol.StartSpec{}, &errs.UsageError{Message: "Command is empty"}
		}
		cmd = tokenized[0]
		procArgs = tokenized[1:]
	} else {
		cmd = p.cmdParts[0]
		procArgs = p.cmdParts[1:]
	}

	if p.runAs == "explicit_user" && p.username == "" {
		return protocol.StartSpec{}, &errs.UsageError{
			Message: "--username is required for explicit_user mode",
		}
	}

	if p.stdio == "file" {
		return protocol.StartSpec{}, &errs.UsageError{
			Message: "stdio 'file' is not supported in CLI yet",
		}
	}

	if p.stdio != "inherit" && p.stdio != "pipe" && p.stdio != "file" {
		return protocol.StartSpec{}, &errs.UsageError{Message: "Invalid stdio mode"}
	}

	envMap := make(map[string]string)
	for _, e := range p.envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return protocol.StartSpec{}, &errs.UsageError{
				Message: "Invalid env format: " + e,
			}
		}
		envMap[parts[0]] = parts[1]
	}

	return protocol.StartSpec{
		Name:  p.name,
		Cmd:   cmd,
		Args:  procArgs,
		Cwd:   p.cwd,
		Env:   envMap,
		Stdio: p.stdio,
		RunAs: protocol.RunAsPolicy{
			Mode:     p.runAs,
			Username: p.username,
		},
	}, nil
}

func printSuccessResponse(data *protocol.StartResponseData, requestedName string) {
	name := requestedName
	if name == "" {
		name = data.ProcID
	}
	fmt.Printf("Started: %s (pid=%d, status=%s)\n",
		term.BoldString("%s", name),
		data.PID,
		term.GreenString("%s", data.Status),
	)
}

func printErrorResponse(err *protocol.StartError) {
	fmt.Printf("%s: %s: %s\n",
		term.RedString("Error"),
		term.BoldString("%s", err.Code),
		err.Message,
	)
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:    "start",
		Aliases: []string{"run"},
		Usage:   term.BoldString("lynx start") + " [options] <cmd> [args...]",
		Description: "Start a new process.\n\n" +
			"Flags can be placed before or after the command.\n" +
			"Arguments after -- are treated as the command and its arguments.\n" +
			"Example: lynx start --name myapp --env PORT=8080 node server.js",
		Options: []help.Option{
			{
				Short:       "",
				Long:        "--name",
				Description: "Process name",
			},
			{
				Short:       "",
				Long:        "--cwd",
				Description: "Working directory",
			},
			{
				Short:       "",
				Long:        "--env",
				Description: "Environment variable (KEY=VALUE). Can be repeated.",
			},
			{
				Short:       "",
				Long:        "--stdio",
				Description: "IO mode: inherit, pipe, file (default: inherit)",
			},
			{
				Short:       "",
				Long:        "--run-as",
				Description: "Execution mode: self, app_user, explicit_user (default: self)",
			},
			{
				Short:       "",
				Long:        "--username",
				Description: "Username for explicit_user mode",
			},
			{
				Short:       "-h",
				Long:        "--help",
				Description: "Show this help message.",
			},
		},
	}
}

// PrintHelp prints the help message for the start command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
