package main

import (
	"fmt"
	"os"

	"github.com/Jaro-c/Lynx/internal/ipc"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lynx <command>")
		os.Exit(1)
	}

	command := os.Args[1]

	if command == "ping" {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to daemon: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			fmt.Fprintf(os.Stderr, "Ping failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Ping response: %v\n", result)
	} else {
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
