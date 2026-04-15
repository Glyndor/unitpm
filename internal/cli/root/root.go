// Package root implements the root command.
//
//nolint:gocyclo,nestif,cyclop
package root

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/commands/apply"
	"github.com/Jaro-c/Lynx/internal/cli/commands/completion"
	deletecmd "github.com/Jaro-c/Lynx/internal/cli/commands/delete"
	"github.com/Jaro-c/Lynx/internal/cli/commands/execenv"
	"github.com/Jaro-c/Lynx/internal/cli/commands/execsandbox"
	"github.com/Jaro-c/Lynx/internal/cli/commands/export"
	"github.com/Jaro-c/Lynx/internal/cli/commands/flush"
	"github.com/Jaro-c/Lynx/internal/cli/commands/installtools"
	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/cli/commands/logs"
	"github.com/Jaro-c/Lynx/internal/cli/commands/monit"
	"github.com/Jaro-c/Lynx/internal/cli/commands/reload"
	"github.com/Jaro-c/Lynx/internal/cli/commands/restart"
	"github.com/Jaro-c/Lynx/internal/cli/commands/show"
	"github.com/Jaro-c/Lynx/internal/cli/commands/start"
	"github.com/Jaro-c/Lynx/internal/cli/commands/startup"
	"github.com/Jaro-c/Lynx/internal/cli/commands/stop"
	"github.com/Jaro-c/Lynx/internal/cli/commands/update"
	"github.com/Jaro-c/Lynx/internal/cli/commands/version"
	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/registry"
	"github.com/Jaro-c/Lynx/internal/term"
)

const (
	cmdList         = "list"
	cmdLogs         = "logs"
	cmdStart        = "start"
	cmdStop         = "stop"
	cmdRestart      = "restart"
	cmdDelete       = "delete"
	cmdStartup      = "startup"
	cmdVersion      = "version"
	cmdApply        = "apply"
	cmdExport       = "export"
	cmdShow         = "show"
	cmdMonit        = "monit"
	cmdReload       = "reload"
	cmdFlush        = "flush"
	cmdUpdate       = "update"
	cmdInstallTools = "install-tools"
	cmdExecEnv      = "_exec-env"
	cmdExecSandbox  = "_exec-sandbox"
	cmdCompletion   = "completion"
	cmdHelp         = "help"
	flagHelp        = "--help"
)

// Execute executes the root CLI command.
func Execute(args []string) int {
	registerCommands()
	specs := registry.GetAll()

	// Strip global flags (--quiet/-q) before command dispatch.
	args = applyGlobalFlags(args)

	if len(args) < 1 {
		help.RenderRootHelp(os.Stdout, specs, true)
		return 0
	}

	cmdName := resolveCommand(args[0])
	if cmdName == "" {
		printError(os.Stderr, "Command not found: %s", args[0])
		help.RenderRootHelp(os.Stderr, specs, true)
		return 1
	}

	if cmdName == "-h" || cmdName == flagHelp {
		help.RenderRootHelp(os.Stdout, specs, true)
		return 0
	}

	if cmdName == cmdHelp {
		if len(args) > 1 {
			subName := resolveCommand(args[1])
			if subName != "" && subName != cmdHelp {
				return printCommandHelp(subName)
			}
			if subName == cmdHelp {
				help.RenderRootHelp(os.Stdout, specs, true)
				return 0
			}
			printError(os.Stderr, "Command not found: %s", args[1])
			help.RenderRootHelp(os.Stderr, specs, true)
			return 1
		}
		help.RenderRootHelp(os.Stdout, specs, true)
		return 0
	}

	// Handle subcommand help
	if len(args) > 1 && isHelpRequest(args[1:]) {
		return printCommandHelp(cmdName)
	}

	err := runCommand(cmdName, args[1:])
	if err != nil {
		handleError(err, cmdName)
		return 1
	}
	return 0
}

func resolveCommand(name string) string {
	if name == "help" {
		return cmdHelp
	}
	if name == "--version" {
		return cmdVersion
	}
	if name == "-h" || name == "--help" {
		return name
	}
	if resolved, found := registry.Resolve(name); found {
		return resolved
	}
	return ""
}

func runCommand(name string, args []string) error {
	switch name {
	case cmdVersion:
		return version.Run(nil, os.Stdout, args)
	case cmdUpdate:
		return update.Run(os.Stdout, args)
	case cmdInstallTools:
		return installtools.Run(args)
	case cmdExport:
		return export.Run(args)
	case cmdExecEnv:
		return execenv.Run(args)
	case cmdExecSandbox:
		return execsandbox.Run(args)
	case cmdCompletion:
		return completion.Run(args)
	case cmdStartup:
		return startup.Run(nil, args)
	case cmdLogs:
		return logs.Run(args)
	case cmdShow:
		return show.Run(nil, args)
	case cmdMonit:
		return monit.Run(nil, args)
	case cmdList:
		return list.Run(nil, args)
	case cmdStart:
		return start.Run(nil, args)
	case cmdApply:
		return apply.Run(nil, args)
	case cmdStop:
		return stop.Run(nil, args)
	case cmdRestart:
		return restart.Run(nil, args)
	case cmdDelete:
		return deletecmd.Run(nil, args)
	case cmdReload:
		return reload.Run(nil, args)
	case cmdFlush:
		return flush.Run(nil, args)
	}
	return nil
}

func printCommandHelp(name string) int {
	switch name {
	case cmdList:
		list.PrintHelp()
	case cmdLogs:
		fmt.Println(logs.GetSpec().Usage)
		fmt.Println(logs.GetSpec().Description)
	case cmdStart:
		start.PrintHelp()
	case cmdStop:
		stop.PrintHelp()
	case cmdRestart:
		restart.PrintHelp()
	case cmdDelete:
		deletecmd.PrintHelp()
	case cmdStartup:
		startup.PrintHelp()
	case cmdVersion:
		version.PrintHelp()
	case cmdUpdate:
		update.PrintHelp()
	case cmdInstallTools:
		installtools.PrintHelp()
	case cmdApply:
		apply.PrintHelp()
	case cmdExport:
		export.PrintHelp()
	case cmdShow:
		show.PrintHelp()
	case cmdMonit:
		monit.PrintHelp()
	case cmdReload:
		reload.PrintHelp()
	case cmdFlush:
		flush.PrintHelp()
	case cmdCompletion:
		completion.PrintHelp()
	}
	return 0
}

func handleError(err error, cmdName string) {
	var usageErr *errs.UsageError
	if errors.As(err, &usageErr) {
		printError(os.Stderr, "%s", usageErr.Message)
		printCommandHelp(cmdName)
	} else {
		printError(os.Stderr, "%v", err)
	}
}

// applyGlobalFlags strips recognized global flags from args and applies them
// as side effects. Currently handles --quiet / -q.
func applyGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--quiet" || a == "-q" {
			term.SetQuiet(true)
			continue
		}
		out = append(out, a)
	}
	return out
}

func isHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printError(w io.Writer, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprintf(w, "%s\n", term.RedString("[Lynx][ERROR] %s", msg))
}

func registerCommands() {
	registry.Register(list.GetSpec())
	registry.Register(logs.GetSpec())
	registry.Register(start.GetSpec())
	registry.Register(stop.GetSpec())
	registry.Register(restart.GetSpec())
	registry.Register(deletecmd.GetSpec())
	registry.Register(startup.GetSpec())
	registry.Register(version.GetSpec())
	registry.Register(update.GetSpec())
	registry.Register(installtools.GetSpec())
	registry.Register(execenv.GetSpec())
	registry.Register(execsandbox.GetSpec())
	registry.Register(completion.GetSpec())
	registry.Register(apply.GetSpec())
	registry.Register(export.GetSpec())
	registry.Register(show.GetSpec())
	registry.Register(monit.GetSpec())
	registry.Register(reload.GetSpec())
	registry.Register(flush.GetSpec())
}
