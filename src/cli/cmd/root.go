package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootCmd is the main command for the Lynx CLI
var rootCmd = &cobra.Command{
	Use: "lynx",
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are provided, display the help message
		if len(args) == 0 {
			title := color.New(color.FgCyan, color.Bold).SprintFunc()
			option := color.New(color.FgYellow).SprintFunc()

			fmt.Println(title("Usage: lynx <command> [flags]\n"))
			fmt.Println(option("Use 'lynx -h' or 'lynx --help' to see all available commands."))
			fmt.Println(option("For specific command help, use 'lynx <command> -h'."))
			return
		}

		_ = cmd.Help()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Commands

	// Help customization
	CustomizeHelp(rootCmd)
}
