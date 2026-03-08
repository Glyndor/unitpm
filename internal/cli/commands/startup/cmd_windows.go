//go:build windows

package startup

import "github.com/Jaro-c/Lynx/internal/cli/help"

// Runner interface allows dependency injection for command execution.
type Runner interface {
	Run(name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

func Run(runner Runner, args []string) error {
	return nil
}

func PrintHelp() {}

func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name: "startup",
	}
}
