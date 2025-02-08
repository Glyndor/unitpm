package internal

import "github.com/fatih/color"

var (
	Color_Title   = color.New(color.FgCyan, color.Bold).SprintFunc()
	Color_Command = color.New(color.FgGreen).SprintFunc()
	Color_Option  = color.New(color.FgYellow).SprintFunc()

	Color_Error   = color.New(color.FgRed, color.Bold).SprintFunc()
	Color_Warning = color.New(color.FgYellow, color.Bold).SprintFunc()
)
