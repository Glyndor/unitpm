//go:build linux

// Package main provides the CLI for interacting with the Lynx daemon.
package main

import (
	"os"

	"github.com/Jaro-c/Lynx/internal/cli/root"
)

func main() {
	os.Exit(root.Execute(os.Args[1:]))
}
