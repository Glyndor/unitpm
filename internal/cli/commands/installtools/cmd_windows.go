//go:build windows

package installtools

import "github.com/Jaro-c/Lynx/internal/cli/help"

func Run(args []string) error {
	return nil
}

func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name: "install-tools",
	}
}

func PrintHelp() {}
