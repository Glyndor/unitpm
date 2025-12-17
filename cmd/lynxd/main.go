package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/ipc"
)

func main() {
	log.Println("lynxd starting...")

	server := ipc.NewServer()

	// Register ping handler
	server.Register("ping", func(params json.RawMessage) (json.RawMessage, error) {
		// Verify params are empty or ignore them?
		// "Validate all parameters"
		// For ping, we expect no parameters or we can just ignore.
		return json.Marshal(map[string]string{"response": "pong"})
	})

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}

	path, _ := ipc.GetSocketPath()
	log.Printf("IPC server listening on %s", path)

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	server.Close()
}
