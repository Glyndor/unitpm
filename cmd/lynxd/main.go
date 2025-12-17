// Package main is the entry point for the lynx daemon.
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
	server.Register("ping", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	// Register list handler (replacing status)
	// Returns a list of processes with their detailed status
	server.Register("list", func(_ json.RawMessage) (json.RawMessage, error) {
		// Mock data for now
		processes := []types.ProcessInfo{
			{
				ID:        0,
				Name:      "web-api",
				Namespace: "default",
				Version:   "1.0.0",
				Mode:      "fork",
				PID:       35711,
				Uptime:    7200000, // 2 hours
				Restarts:  3,
				State:     types.StateOnline,
				CPU:       0.0,
				Memory:    10066329, // ~10MB
				User:      "svc-web",
				Watch:     false,
			},
			{
				ID:        1,
				Name:      "worker-queue",
				Namespace: "default",
				Version:   "1.0.2",
				Mode:      "cluster",
				PID:       0,
				Uptime:    0,
				Restarts:  10,
				State:     types.StateStopped,
				CPU:       0.0,
				Memory:    0,
				User:      "root",
				Watch:     true,
			},
			{
				ID:        2,
				Name:      "db-proxy",
				Namespace: "db",
				Version:   "0.5.0",
				Mode:      "fork",
				PID:       0,
				Uptime:    300000, // 5 mins
				Restarts:  0,
				State:     types.StateFailed,
				CPU:       0.0,
				Memory:    0,
				User:      "db-user",
				Watch:     false,
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
	_ = server.Close()
}
