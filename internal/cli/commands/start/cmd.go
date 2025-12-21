// Package start implements the start command.
package start

import (
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

	spec, err := parseStartSpec(args)
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
			return fmt.Errorf("process start failed")
		}
		return fmt.Errorf("process start failed with unknown error")
	}

	if startResp.Data != nil {
		printSuccessResponse(startResp.Data, spec.Name)
	}

	return nil
}

func parseStartSpec(args []string) (protocol.StartSpec, error) {
	var (
		name     string
		cwd      string
		stdio    = "inherit"
		runAs    = "self"
		username string
		envs     []string
		cmdParts []string
	)

	parsingFlags := true
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !parsingFlags {
			cmdParts = append(cmdParts, arg)
			continue
		}

		if arg == "--" {
			parsingFlags = false
			continue
		}

		if strings.HasPrefix(arg, "-") {
			// Handle flags
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

			// Clean dashes
			flagName = strings.TrimLeft(flagName, "-")

			// Helper to get value
			getValue := func() (string, error) {
				if hasValue {
					return flagValue, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("flag --%s requires a value", flagName)
				}
				i++ // consume next arg
				return args[i], nil
			}

			switch flagName {
			case "name":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				name = val
			case "cwd":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				cwd = val
			case "stdio":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				stdio = val
			case "run-as":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				runAs = val
			case "username":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				username = val
			case "env":
				val, err := getValue()
				if err != nil {
					return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
				}
				envs = append(envs, val)
			case "cron":
				// Fail fast for cron
				return protocol.StartSpec{}, &errs.UsageError{Message: "ERR_UNSUPPORTED: cron scheduling is not implemented yet"}
			default:
				// If it looks like a flag but we don't recognize it, it might be part of the command if it's not a known flag.
				// However, standard CLI behavior usually errors on unknown flags unless we are sure it's an arg.
				// But PM2 allows "pm2 start app.js -- arg1 arg2".
				// Requirement: "Flags may appear before or after the command."
				// Requirement: "First non-flag tokens form the command"
				// If we encounter an unknown flag, treat it as an error or command part?
				// "lynx start node --run dev" -> "node" is cmd, "--run", "dev" are args.
				// "--run" starts with "-".
				// If we are strictly parsing flags for lynx, any unknown flag should probably be treated as part of the command?
				// But if it's before the command?
				// "lynx start --unknown-flag cmd" -> Should this fail or run "--unknown-flag" as command?
				// Usually fails.
				// But "lynx start cmd --arg" -> "--arg" is arg to cmd.
				
				// Let's refine the logic:
				// We need to identify if we have found the command yet.
				// Actually, the requirement says: "First non-flag tokens form the command"
				// This implies that flags must be known flags to be consumed.
				// If it's not a known flag, it's a token.
				
				cmdParts = append(cmdParts, arg)
			}
			continue
		}

		// Not a flag
		cmdParts = append(cmdParts, arg)
	}

	if len(cmdParts) == 0 {
		return protocol.StartSpec{}, &errs.UsageError{Message: "Command is required"}
	}

	var cmd string
	var procArgs []string

	// Command Resolution Logic
	// If the command is one single token containing spaces (quoted by user), treat it as a cmdline string
	if len(cmdParts) == 1 && strings.Contains(cmdParts[0], " ") {
		// Use lexer
		tokenized, err := tokenize(cmdParts[0])
		if err != nil {
			return protocol.StartSpec{}, &errs.UsageError{Message: fmt.Sprintf("Failed to parse command line: %v", err)}
		}
		if len(tokenized) == 0 {
			return protocol.StartSpec{}, &errs.UsageError{Message: "Command is empty"}
		}
		cmd = tokenized[0]
		procArgs = tokenized[1:]
	} else {
		cmd = cmdParts[0]
		procArgs = cmdParts[1:]
	}

	// Validation
	if runAs == "explicit_user" && username == "" {
		return protocol.StartSpec{}, &errs.UsageError{Message: "--username is required for explicit_user mode"}
	}

	if stdio == "file" {
		return protocol.StartSpec{}, &errs.UsageError{Message: "stdio 'file' is not supported in CLI yet"}
	}

	if stdio != "inherit" && stdio != "pipe" && stdio != "file" {
		return protocol.StartSpec{}, &errs.UsageError{Message: "Invalid stdio mode"}
	}

	// Construct Env map
	envMap := make(map[string]string)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return protocol.StartSpec{}, &errs.UsageError{Message: fmt.Sprintf("Invalid env format: %s", e)}
		}
		envMap[parts[0]] = parts[1]
	}

	return protocol.StartSpec{
		Name:  name,
		Cmd:   cmd,
		Args:  procArgs,
		Cwd:   cwd,
		Env:   envMap,
		Stdio: stdio,
		RunAs: protocol.RunAsPolicy{
			Mode:     runAs,
			Username: username,
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
			{Short: "", Long: "--name", Description: "Process name"},
			{Short: "", Long: "--cwd", Description: "Working directory"},
			{Short: "", Long: "--env", Description: "Environment variable (KEY=VALUE). Can be repeated."},
			{Short: "", Long: "--stdio", Description: "IO mode: inherit, pipe, file (default: inherit)"},
			{Short: "", Long: "--run-as", Description: "Execution mode: self, app_user, explicit_user (default: self)"},
			{Short: "", Long: "--username", Description: "Username for explicit_user mode"},
			{Short: "-h", Long: "--help", Description: "Show this help message."},
		},
	}
}

// PrintHelp prints the help message for the start command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
