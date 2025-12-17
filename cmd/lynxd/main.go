package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/types"
)

func main() {
	log.Println("lynxd starting...")

	server := ipc.NewServer()

	// Register ping handler
	server.Register("ping", func(params json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	// Register status handler
	// Returns a list of processes with their status
	server.Register("status", func(params json.RawMessage) (json.RawMessage, error) {
		// Mock data for now
		processes := []types.ProcessInfo{
			{
				Name:   "web-api",
				State:  types.StateRunning,
				PID:    1234,
				Uptime: "2h 15m",
				Memory: "128MB",
				CPU:    "0.5%",
			},
			{
				Name:   "worker-queue",
				State:  types.StateStopped,
				Uptime: "0s",
			},
			{
				Name:   "db-proxy",
				State:  types.StateFailed,
				PID:    0,
				Uptime: "5m", // Maybe it failed 5m ago
			},
		}
		return json.Marshal(processes)
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
