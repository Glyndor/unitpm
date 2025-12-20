// Package start implements the start command.
package start

import (
	"flag"
	"fmt"
	"io"
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
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		name     string
		cwd      string
		stdio    string
		runAs    string
		username string
		envs     envFlag
	)

	fs.StringVar(&name, "name", "", "Process name")
	fs.StringVar(&cwd, "cwd", "", "Working directory")
	fs.StringVar(&stdio, "stdio", "inherit", "IO mode (inherit|pipe|file)")
	fs.StringVar(&runAs, "run-as", "self", "Execution mode (self|app_user|explicit_user)")
	fs.StringVar(&username, "username", "", "Username for explicit_user mode")
	fs.Var(&envs, "env", "Environment variable (KEY=VALUE)")

	if err := fs.Parse(args); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			flagName := strings.TrimPrefix(err.Error(), "flag provided but not defined: -")
			return protocol.StartSpec{}, &errs.UsageError{Message: "Unknown flag: -" + flagName}
		}
		return protocol.StartSpec{}, &errs.UsageError{Message: err.Error()}
	}

	// Parsing Command and Args
	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		return protocol.StartSpec{}, &errs.UsageError{Message: "Command is required"}
	}

	cmd := cmdArgs[0]
	procArgs := cmdArgs[1:]

	// Validation
	if runAs == "explicit_user" && username == "" {
		return protocol.StartSpec{}, &errs.UsageError{Message: "--username is required for explicit_user mode"}
	}

	if stdio == "file" {
		// Need file path option, not implemented yet as per requirements
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

// envFlag implements flag.Value for repeatable flags
type envFlag []string

func (e *envFlag) String() string {
	return strings.Join(*e, ",")
}

func (e *envFlag) Set(value string) error {
	*e = append(*e, value)
	return nil
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:    "start",
		Aliases: []string{"run"},
		Usage:   term.BoldString("lynx start") + " [options] -- <cmd> [args...]",
		Description: "Start a new process.\n\n" +
			"Arguments after -- are treated as the command and its arguments.\n" +
			"Example: lynx start --name myapp --env PORT=8080 -- ./server -c config.json",
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
