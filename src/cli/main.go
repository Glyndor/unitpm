package main

import (
	"log"
	"lynx/cmd"
)

func main() {
	// Execute CLI commands
	if err := cmd.Execute(); err != nil {
		log.Fatal("Error:", err)
	}
}
