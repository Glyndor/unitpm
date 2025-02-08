package root

import (
	"fmt"
	"lynx/internal"

	"github.com/spf13/cobra"
)

// rootCmd is the main command for the Lynx CLI
var rootCmd = &cobra.Command{
	Use: "lynx",
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are provided, display the help message
		if len(args) == 0 {
			fmt.Println(internal.Color_Title("Usage: lynx <command> [flags]\n"))
			fmt.Println(internal.Color_Option("Use 'lynx -h' or 'lynx --help' to see all available commands."))
			fmt.Println(internal.Color_Option("For specific command help, use 'lynx <command> -h'."))
			return
		}

		_ = cmd.Help()
	},
}

// Execute runs the CLI
func Execute() error {
	if error := internal.Ensure_LynxHome(); error != nil {
		internal.Error_Fatal(error)
	}

	return rootCmd.Execute()
}

func init() {
	// Commands

	// Help customization
	CustomizeHelp(rootCmd)
}
