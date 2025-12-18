// Package main provides the CLI for interacting with the Lynx daemon.
package main

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/root"
	"github.com/Jaro-c/Lynx/internal/term"
)

func main() {
	if err := root.Execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", term.RedString("%v", err))
		os.Exit(1)
	}
}
