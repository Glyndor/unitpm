package main

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lynx <command>")
		os.Exit(1)
	}

	command := os.Args[1]

	// Use package-level convenience functions which use the global default Styler
	if command == "ping" {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Failed to connect to daemon: %v", err))
			os.Exit(1)
		}
		defer client.Close()

		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Ping failed: %v", err))
			os.Exit(1)
		}

		fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))
	} else {
		fmt.Printf("%s\n", term.YellowString("Unknown command: %s", command))
		os.Exit(1)
	}
}
