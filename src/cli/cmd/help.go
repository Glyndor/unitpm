package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// PrintHelpMessage prints the help message for a command
func PrintHelpMessage(cmd *cobra.Command) {
	title := color.New(color.FgCyan, color.Bold).SprintFunc()
	command := color.New(color.FgGreen).SprintFunc()
	option := color.New(color.FgYellow).SprintFunc()

	// Print Usage
	fmt.Println(title("Usage: lynx <command> [flags]\n"))

	// Print Available Commands
	fmt.Println(title("Available Commands:"))
	for _, c := range cmd.Commands() {
		if !c.Hidden {
			fmt.Printf("  %s  %s\n", command(c.Name()), c.Short)
		}
	}

	// Print Options
	fmt.Println(title("\nOptions:"))
	fmt.Printf("  %s  %s\n", option("-h, --help"), "Show all available commands and options")

	// Print Help Footer
	fmt.Println(title("\nFor specific command help, use 'lynx <command> -h'."))
}

// CustomizeHelp customizes the help message for a command
func CustomizeHelp(cmd *cobra.Command) {
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		PrintHelpMessage(cmd)
	})
}
