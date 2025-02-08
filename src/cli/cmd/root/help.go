package root

import (
	"fmt"
	"lynx/internal"

	"github.com/spf13/cobra"
)

func help_message(cmd *cobra.Command) {
	// Usage
	fmt.Println(internal.Color_Title("Usage: lynx <command> [flags]\n"))

	// Available Commands
	fmt.Println(internal.Color_Title("Available Commands:"))
	for _, c := range cmd.Commands() {
		if !c.Hidden {
			fmt.Printf("  %s  %s\n", internal.Color_Command(c.Name()), c.Short)
		}
	}

	// Options
	fmt.Println(internal.Color_Title("\nOptions:"))
	fmt.Printf("  %s  %s\n", internal.Color_Option("-h, --help"), "Show all available commands and options")

	// Help Footer
	fmt.Println(internal.Color_Title("\nFor specific command help, use 'lynx <command> -h'."))
}

// Customizes the help message for a command
func CustomizeHelp(cmd *cobra.Command) {
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		help_message(cmd)
	})
}
