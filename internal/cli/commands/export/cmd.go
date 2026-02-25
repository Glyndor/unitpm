//go:build linux

package export

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/lynxfile"
	"github.com/Jaro-c/Lynx/internal/spec"
	"gopkg.in/yaml.v3"
)

// Run executes the export command to generate a Lynxfile from currently running applications.
func Run(args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	if len(args) < 2 {
		return errs.NewUsageError("export requires --namespace <name>")
	}

	var namespace string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--namespace" || arg == "-n" {
			if i+1 >= len(args) {
				return errs.NewUsageError("missing value for --namespace")
			}
			namespace = args[i+1]
			i++
			continue
		}
	}

	if namespace == "" {
		return errs.NewUsageError("missing --namespace")
	}

	specs, err := spec.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load specs: %w", err)
	}

	file := lynxfile.File{
		Version:   "1",
		Namespace: namespace,
	}

	for _, s := range specs {
		ns := s.Namespace
		if ns == "" {
			ns = "default"
		}
		if ns != namespace {
			continue
		}

		app := lynxfile.AppConfig{
			Name: s.Name,
			Cwd:  s.Cwd,
			Env:  s.Env,
		}

		if s.Exec.Type == "command" {
			cmd := s.Exec.Command
			if len(s.Exec.Args) > 0 {
				for _, a := range s.Exec.Args {
					cmd += " " + a
				}
			}
			app.Command = cmd
		} else if s.Exec.Type == "entry" {
			app.Entry = s.Exec.Entry
			app.Runtime = s.Exec.Runtime
		}

		if s.Logs != nil {
			app.Logs = lynxfile.LogsConfig{
				Dir:       s.Logs.Dir,
				Stdout:    s.Logs.Stdout,
				Stderr:    s.Logs.Stderr,
				Format:    s.Logs.Format,
				Timestamp: s.Logs.Timestamp,
			}
		}

		if s.Restart != nil {
			app.Restart = lynxfile.RestartConfig{
				Policy:      s.Restart.Policy,
				MaxRestarts: s.Restart.MaxRetries,
				DelayMs:     s.Restart.BackoffMs,
				Backoff:     s.Restart.BackoffType,
				StopOnExit:  s.Restart.StopOnExit,
			}
		}

		file.Apps = append(file.Apps, app)
	}

	out, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to encode Lynxfile: %w", err)
	}

	_, err = os.Stdout.Write(out)
	return err
}

// GetSpec returns the command specification for the export command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "export",
		Usage:       "lynx export --namespace <name>",
		Description: "Export current applications in a namespace to Lynxfile.yml format",
	}
}

// PrintHelp prints the help information for the export command.
func PrintHelp(w ...interface{}) {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
